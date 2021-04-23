package pb

import (
	"fmt"
	"io"
	"os"
)

// PacketReceiver is anything capable of returning a Packet.
type PacketReceiver interface {
	Recv() (*Packet, error)
}

// PacketSender is something capable of sending a Packet struct.
type PacketSender interface {
	Send(*Packet) error
}

// ProgressTracker is the interface used to track progress during a send (push) or
// receive (pull) of a file. SetMax is called only once and prior to and SetCurrent
// call.
type ProgressTracker interface {
	SetMax(int64)
	SetCurrent(int64)
}

// SendProgressMessage sends a progress message down the protobuf connection.
// This message contains the total file size and the current offset.
func SendProgressMessage(offset int64, size int64, sender PacketSender) error {
	return sender.Send(
		&Packet{
			TestOneof: &Packet_Progress{
				Progress: &Progress{
					Offset: offset,
					Size:   size,
				},
			},
		},
	)
}

// Receive receives Packets from provided PacketReceiver and writes their content into
// a provided Writer. Progress is reported through a ProgressTracker.
func Receive(from PacketReceiver, to io.Writer, tracker ProgressTracker) error {
	var fsize int64
	var tracktotal bool
	for {
		in, err := from.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error receiving chunk: %w", err)
		}

		// check if the received message is a progress indication, notify
		// the provided ProgressTracker and moves on to the next message.
		progress := in.GetProgress()
		if progress != nil {
			if !tracktotal {
				tracker.SetMax(progress.Size)
				tracktotal = true
			}
			tracker.SetCurrent(int64(progress.Offset))
			continue
		}

		ck := in.GetChunk()
		if ck == nil {
			return fmt.Errorf("nil chunk received")
		}

		written, err := to.Write(ck.Content)
		if err != nil {
			return fmt.Errorf("error writing to temp file: %w", err)
		}
		fsize += int64(written)
	}
	tracker.SetCurrent(fsize)
	return nil
}

// Send sends a file from disk through a PacketSender. File is split into chunks of 1024 bytes.
// From time to time this function also sends over the wire a progress message, informing the
// total file size and the current offset.
func Send(from *os.File, to PacketSender, tracker ProgressTracker) error {
	finfo, err := from.Stat()
	if err != nil {
		return fmt.Errorf("error getting file info: %s", err)
	}
	fsize := finfo.Size()
	tracker.SetMax(finfo.Size())

	var counter int
	var totread int64
	for {
		content := make([]byte, 1024)
		read, err := from.Read(content)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading file: %w", err)
		}
		totread += int64(read)

		if counter%50 == 0 {
			if err := SendProgressMessage(totread, fsize, to); err != nil {
				return fmt.Errorf("error sending progress: %w", err)
			}
		}

		if err := to.Send(
			&Packet{
				TestOneof: &Packet_Chunk{
					Chunk: &Chunk{
						Content: content,
					},
				},
			},
		); err != nil {
			return fmt.Errorf("error sending chunk: %w", err)
		}
		tracker.SetCurrent(totread)
		counter++
	}
	return nil
}
