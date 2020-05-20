package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"os"
	"path/filepath"
)

const chunkStartOffset = 8
const endChunk = "IEND"

type pngChunk struct {
	Offset int64
	Length uint32
	Type   [4]byte
	Data   []byte
	CRC    uint32
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("must provide a root directory")
	}

	rootDir := os.Args[1]

	if len(rootDir) < 10 {
		log.Fatal("invalid root directory")
	}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}

		file, err := os.OpenFile(path, os.O_RDWR, 0666)
		if err != nil {
			return err
		}

		defer file.Close()

		if !isPng(file) {
			return nil
		}

		// Read all the chunks. They start with IHDR at offset 8
		chunks := readChunks(file)
		corrected := false
		for _, chunk := range chunks {
			if !chunk.CRCIsValid() {
				corrected = true
				file.Seek(chunk.CRCOffset(), os.SEEK_SET)
				binary.Write(file, binary.BigEndian, chunk.CalculateCRC())
			}

		}

		if corrected {
			fmt.Println("Corrected CRC in", path)
		}

		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

}

func (p pngChunk) String() string {
	return fmt.Sprintf("%s@%x - %X - Valid CRC? %v", p.Type, p.Offset, p.CRC, p.CRCIsValid())
}

func (p pngChunk) Bytes() []byte {
	var buffer bytes.Buffer

	binary.Write(&buffer, binary.BigEndian, p.Type)
	buffer.Write(p.Data)

	return buffer.Bytes()
}

func (p pngChunk) CRCIsValid() bool {
	return p.CRC == p.CalculateCRC()
}

func (p pngChunk) CalculateCRC() uint32 {
	crcTable := crc32.MakeTable(crc32.IEEE)

	return crc32.Checksum(p.Bytes(), crcTable)
}

func (p pngChunk) CRCOffset() int64 {
	return p.Offset + int64(8+p.Length)
}

func readChunks(reader io.ReadSeeker) []pngChunk {
	chunks := []pngChunk{}

	reader.Seek(chunkStartOffset, os.SEEK_SET)

	readChunk := func() (*pngChunk, error) {
		var chunk pngChunk
		chunk.Offset, _ = reader.Seek(0, os.SEEK_CUR)

		binary.Read(reader, binary.BigEndian, &chunk.Length)

		chunk.Data = make([]byte, chunk.Length)

		err := binary.Read(reader, binary.BigEndian, &chunk.Type)
		if err != nil {
			goto read_error
		}

		if read, err := reader.Read(chunk.Data); read == 0 || err != nil {
			goto read_error
		}

		err = binary.Read(reader, binary.BigEndian, &chunk.CRC)
		if err != nil {
			goto read_error
		}

		return &chunk, nil

	read_error:
		return nil, fmt.Errorf("Read error")
	}

	chunk, err := readChunk()
	if err != nil {
		return chunks
	}

	chunks = append(chunks, *chunk)

	// Read the first chunk
	for string(chunks[len(chunks)-1].Type[:]) != endChunk {

		chunk, err := readChunk()
		if err != nil {
			break
		}

		chunks = append(chunks, *chunk)
	}

	return chunks
}

func isPng(f *os.File) bool {
	f.Seek(1, os.SEEK_SET)

	magic := make([]byte, 3)
	f.Read(magic)

	return string(magic) == "PNG"
}
