package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

const (
	pcmpDataOffset = 0x20 // Start of compressed data in PCMP file
)

var (
	errFileTooShort        = errors.New("pcmp: file too short for PCMP header")
	errInvalidSignature    = errors.New("pcmp: 'PCMP' signature not found")
	errStreamEmpty         = errors.New("lzss: compressed stream is empty")
	errStreamPrematureEnd  = errors.New("lzss: compressed stream ended prematurely")
	errPrematureEndCopy    = errors.New("lzss: premature end while reading copy block")
	errPrematureEndLiteral = errors.New("lzss: premature end while reading literal")
)

func readPCMPHeader(buf []byte) (outSize, compSize uint32, err error) {
	if len(buf) < pcmpDataOffset {
		return 0, 0, errFileTooShort
	}
	if string(buf[:4]) != "PCMP" {
		return 0, 0, errInvalidSignature
	}

	outSize = binary.LittleEndian.Uint32(buf[0x14:0x18])
	compSize = binary.LittleEndian.Uint32(buf[0x18:0x1C])

	remainingDataInFile := uint32(len(buf) - pcmpDataOffset)
	if compSize == 0 || compSize > remainingDataInFile {
		compSize = remainingDataInFile
		fmt.Fprintf(os.Stderr, "Warning: comp_size in header is invalid or exceeds file bounds. Using remaining file size (%d bytes) for compressed stream.\n", compSize)
	}
	return outSize, compSize, nil
}

func lzssSimpleDecompress(compressedStream []byte, uncompressedSize uint32) ([]byte, error) {
	outBuffer := make([]byte, 0, uncompressedSize) // Pre-allocate with expected size
	streamIdx := 0
	streamLen := len(compressedStream)

	if streamLen == 0 {

		if uncompressedSize > 0 {
			return nil, errStreamEmpty
		}
		return []byte{}, nil
	}

	controlByte := compressedStream[streamIdx]
	streamIdx++
	bitsLeft := 8

	for uint32(len(outBuffer)) < uncompressedSize {
		if streamIdx > streamLen {
			return nil, errStreamPrematureEnd
		}

		isCopyOperation := (controlByte & 0x80) != 0 // Check MSB
		controlByte <<= 1
		bitsLeft--

		if bitsLeft == 0 {
			if streamIdx >= streamLen {
				if uint32(len(outBuffer)) < uncompressedSize {
					return nil, errStreamPrematureEnd
				}
				break
			}
			controlByte = compressedStream[streamIdx]
			streamIdx++
			bitsLeft = 8
		}

		if isCopyOperation {
			if streamIdx+1 >= streamLen {
				return nil, errPrematureEndCopy
			}
			byte1 := compressedStream[streamIdx]
			byte2 := compressedStream[streamIdx+1]
			streamIdx += 2

			offset := int(((uint16(byte1) >> 4) | (uint16(byte2) << 4))) + 1
			count := int((byte1 & 0x0F)) + 3

			for i := 0; i < count; i++ {
				if uint32(len(outBuffer)) >= uncompressedSize {
					break
				}
				if offset > len(outBuffer) {
					outBuffer = append(outBuffer, 0)
				} else {
					outBuffer = append(outBuffer, outBuffer[len(outBuffer)-offset])
				}
			}
		} else { // Literal operation
			if streamIdx >= streamLen {
				return nil, errPrematureEndLiteral
			}
			if uint32(len(outBuffer)) >= uncompressedSize {

				break
			}
			outBuffer = append(outBuffer, compressedStream[streamIdx])
			streamIdx++
		}
	}

	if uint32(len(outBuffer)) != uncompressedSize {

		fmt.Fprintf(os.Stderr, "Warning: Decompressed size (%d) does not match expected size (%d). Stream might be truncated or corrupted.\n", len(outBuffer), uncompressedSize)
	}

	return outBuffer, nil
}

func decompressPCMPFile(filePath string) ([]byte, error) {
	fileData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", filePath, err)
	}

	uncompressedSize, compressedStreamSize, err := readPCMPHeader(fileData)
	if err != nil {
		return nil, fmt.Errorf("processing PCMP header: %w", err)
	}

	fmt.Printf("PCMP Header: Uncompressed Size: %d, Compressed Stream Size: %d\n", uncompressedSize, compressedStreamSize)

	if pcmpDataOffset+int(compressedStreamSize) > len(fileData) {
		return nil, fmt.Errorf("compressed stream size (%d) + offset (%d) exceeds file size (%d)", compressedStreamSize, pcmpDataOffset, len(fileData))
	}
	if compressedStreamSize == 0 && uncompressedSize > 0 {
		return nil, fmt.Errorf("header indicates compressed stream size is 0 but uncompressed size is %d", uncompressedSize)
	}
	if compressedStreamSize == 0 && uncompressedSize == 0 {
		fmt.Println("File is empty (0 uncompressed size, 0 compressed stream size).")
		return []byte{}, nil
	}

	compressedStream := fileData[pcmpDataOffset : pcmpDataOffset+int(compressedStreamSize)]

	fmt.Println("Decompressing LZSS stream...")
	decompressedData, err := lzssSimpleDecompress(compressedStream, uncompressedSize)
	if err != nil {
		return nil, fmt.Errorf("LZSS decompression: %w", err)
	}
	fmt.Println("Decompression successful.")
	return decompressedData, nil
}

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s <inputFile.pcmp> [outputFile.bin]\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "If outputFile is not specified, it will be <inputFile_basename>.bin\n")
		os.Exit(1)
	}

	inputFile := args[0]
	outputFile := ""
	if len(args) > 1 {
		outputFile = args[1]
	} else {
		base := strings.TrimSuffix(inputFile, filepath.Ext(inputFile))
		if strings.ToLower(filepath.Ext(base)) == ".txb" && strings.ToLower(filepath.Ext(inputFile)) == ".pcmp" {
			outputFile = base + ".bin"
		} else if strings.ToLower(filepath.Ext(base)) == ".pcmp" {
			outputFile = base + ".bin"
		} else {
			outputFile = base + ".bin"
		}
	}

	fmt.Printf("Input PCMP file: %s\n", inputFile)
	fmt.Printf("Output decompressed file: %s\n", outputFile)

	decompressedData, err := decompressPCMPFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(outputFile, decompressedData, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file '%s': %v\n", outputFile, err)
		os.Exit(1)
	}

	fmt.Printf("Successfully decompressed '%s' to '%s'\n", inputFile, outputFile)
}
