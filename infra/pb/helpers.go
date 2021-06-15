// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pb

import (
	"fmt"
	"io"
)

// PacketReceiver is anything capable of receiving a Packet.
type PacketReceiver interface {
	Recv() (*Packet, error)
}

// PacketSender is something capable of sending a Packet struct.
type PacketSender interface {
	Send(*Packet) error
}

// ProgressTracker is the interface used to track progress during a send or
// receive of a file. SetMax is called only once and prior to any SetCurrent
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
// the provided Writer. Progress is reported through a ProgressTracker.
func Receive(from PacketReceiver, to io.Writer, tracker ProgressTracker) error {
	var fsize int64
	var tracktotal bool
	for {
		in, err := from.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error receiving packet: %w", err)
		}

		// check if the received message is a progress indication, notify
		// the provided ProgressTracker and moves on to the next message.
		progress := in.GetProgress()
		if progress != nil {
			if !tracktotal {
				tracker.SetMax(progress.Size)
				tracktotal = true
			}
			tracker.SetCurrent(progress.Offset)
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

// Send sends contents of provided Reader through a PacketSender. Content is split
// into chunks of 1024 bytes. From time to time this function also sends over the
// wire a progress message, informing the total file size and the current offset.
func Send(
	from io.Reader, totalSize int64, to PacketSender, tracker ProgressTracker,
) error {
	var counter int
	var totread int64
	for {
		content := make([]byte, 1024)
		read, err := from.Read(content)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading: %w", err)
		}
		totread += int64(read)

		if counter%50 == 0 {
			if err := SendProgressMessage(totread, totalSize, to); err != nil {
				return fmt.Errorf("error sending progress: %w", err)
			}
		}

		pckt := &Packet{
			TestOneof: &Packet_Chunk{
				Chunk: &Chunk{
					Content: content,
				},
			},
		}
		if err := to.Send(pckt); err != nil {
			return fmt.Errorf("error sending chunk: %w", err)
		}

		tracker.SetCurrent(totread)
		counter++
	}
	return nil
}
