package pb

import (
	"fmt"
	"io"
	"os"

	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
)

// SendProgressMessage sends a progress message down the protobuf connection.
// This message contains the total file size and the current offset.
func SendProgressMessage(offset uint64, size int64, stream TagIOService_PullServer) error {
	return stream.Send(
		&PullResult{
			TestOneof: &PullResult_Progress{
				Progress: &Progress{
					Offset: offset,
					Size:   size,
				},
			},
		},
	)
}

// ReceiveFileServer reads messages from stream and write its content into provided
// file descriptor.
func ReceiveFileServer(to *os.File, stream TagIOService_PushServer) (int, error) {
	var fsize int
	for {
		in, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("error receiving chunk: %w", err)
		}

		ck := in.GetChunk()
		if ck == nil {
			return 0, fmt.Errorf("nil chunk received")
		}

		written, err := to.Write(ck.Content)
		if err != nil {
			return 0, fmt.Errorf("error writing to temp file: %w", err)
		}
		fsize += written
	}
	return fsize, nil
}

// SendFileServer sends a file from disk through a pull grpc server. File is
// split into chunks of 1024 bytes. From time to time this function also
// sends over the wire a progress message, informing the total file size
// and the current offset.
func SendFileServer(from *os.File, stream TagIOService_PullServer) error {
	finfo, err := from.Stat()
	if err != nil {
		return fmt.Errorf("error getting file info: %s", err)
	}
	fsize := finfo.Size()

	var counter int
	var totread uint64
	for {
		content := make([]byte, 1024)
		read, err := from.Read(content)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error reading file: %w", err)
		}
		totread += uint64(read)

		if counter%50 == 0 {
			err := SendProgressMessage(totread, fsize, stream)
			if err != nil {
				return fmt.Errorf("error sending progress: %w", err)
			}
		}

		if err := stream.Send(
			&PullResult{
				TestOneof: &PullResult_Chunk{
					Chunk: &Chunk{
						Content: content,
					},
				},
			},
		); err != nil {
			return fmt.Errorf("error sending chunk: %w", err)
		}
		counter++
	}
	return nil
}

// ReceiveFileClient reads messages from stream and writes its content into provided
// file descriptor.
func ReceiveFileClient(to *os.File, stream TagIOService_PullClient) (int, error) {
	var bar *mpb.Bar
	prg := mpb.New(mpb.WithWidth(60))
	defer prg.Wait()

	var fsize int
	var tsize int64
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			return 0, err
		}

		if progress := resp.GetProgress(); progress != nil {
			if bar == nil {
				tsize = progress.Size
				bar = prg.Add(
					progress.Size,
					mpb.NewBarFiller(" ▮▮▯ "),
					mpb.PrependDecorators(decor.Name("Pulling")),
					mpb.AppendDecorators(decor.CountersKiloByte("%d %d")),
				)
			}
			bar.SetCurrent(int64(progress.Offset))
			continue
		}

		chunk := resp.GetChunk()
		written, err := to.Write(chunk.Content)
		if err != nil {
			return 0, err
		}
		fsize += written
	}

	if bar != nil {
		bar.SetCurrent(tsize)
	}

	if _, err := to.Seek(0, 0); err != nil {
		return 0, err
	}

	return fsize, nil
}

// SendFileClient reads file descriptor and sends over its content through
// the provided GRPC client.
func SendFileClient(from *os.File, stream TagIOService_PushClient) (int, error) {
	prg := mpb.New(mpb.WithWidth(60))
	defer prg.Wait()

	finfo, err := from.Stat()
	if err != nil {
		return 0, err
	}

	bar := prg.Add(
		finfo.Size(),
		mpb.NewBarFiller(" ▮▮▯ "),
		mpb.PrependDecorators(decor.Name("Pushing")),
		mpb.AppendDecorators(decor.CountersKiloByte("%d %d")),
	)

	var written int
	for {
		content := make([]byte, 1024)
		read, err := from.Read(content)
		if err == io.EOF {
			if _, err := stream.CloseAndRecv(); err != nil {
				return 0, err
			}
			break
		} else if err != nil {
			return 0, err
		}

		ireq := &PushRequest{
			TestOneof: &PushRequest_Chunk{
				Chunk: &Chunk{
					Content: content,
				},
			},
		}
		if err := stream.Send(ireq); err != nil {
			return 0, err
		}
		bar.IncrBy(read)
		written += read
	}
	return written, nil
}
