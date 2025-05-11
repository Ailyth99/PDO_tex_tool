package main

import (
	"bytes"
	"encoding/binary"
	"errors"

	//"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxWindowSize     = 4096
	maxEncodedLength  = 18
	minMatchLength    = 3
	pcmpHeaderSize    = 0x20 // PCMP header is 32 bytes
	outputBufferSlack = 2048 // Extra space for output buffer heuristic
	fileAlignment     = 2048 // Alignment required
)

var errBufferTooSmall = errors.New("pcmp: compressed size exceeds allocated buffer heuristic")

// PCMP Header
func writePCMPHeader(totalUncompressedSize, totalCompressedStreamSize uint32) ([]byte, error) {
	header := make([]byte, pcmpHeaderSize)
	copy(header[0:4], []byte("PCMP"))
	for i := 4; i < pcmpHeaderSize; i++ {
		if i >= 0x14 && i < 0x1C {
			continue
		}
		header[i] = 0x00
	}
	// header[4] = 0x01 // Uncomment if byte 0x04 must be 0x01

	binary.LittleEndian.PutUint32(header[0x14:0x18], totalUncompressedSize)
	binary.LittleEndian.PutUint32(header[0x18:0x1C], totalCompressedStreamSize)
	return header, nil
}

// LZSS Match Finder (Max Offset Priority!!)
func findMatch(inputData []byte, currentInputPos int, currentWindowSize int, maxMatchLen int) (offset int, length int, found bool) {
	if currentInputPos+minMatchLength > len(inputData) {
		return 0, 0, false
	}
	if currentInputPos+maxMatchLen > len(inputData) {
		maxMatchLen = len(inputData) - currentInputPos
	}
	if maxMatchLen < minMatchLength {
		return 0, 0, false
	}
	windowStart := currentInputPos - currentWindowSize
	if windowStart < 0 {
		windowStart = 0
	}
	window := inputData[windowStart:currentInputPos]
	if len(window) == 0 {
		return 0, 0, false
	}

	for l := maxMatchLen; l >= minMatchLength; l-- {
		if currentInputPos+l > len(inputData) {
			continue
		}
		pattern := inputData[currentInputPos : currentInputPos+l]
		idxInWindow := bytes.LastIndex(window, pattern)
		if idxInWindow != -1 {
			matchOffset := currentInputPos - (windowStart + idxInWindow)
			return matchOffset, l, true
		}
	}
	return 0, 0, false
}

// Compressor
func coreCompressV1(inputData []byte) ([]byte, error) {
	if len(inputData) == 0 {
		return []byte{}, nil
	}
	estimatedOutputSize := len(inputData) + outputBufferSlack
	if len(inputData) < 1024 {
		estimatedOutputSize = len(inputData)*2 + 128
	}
	if estimatedOutputSize < 128 {
		estimatedOutputSize = 128
	}
	outputBuffer := make([]byte, estimatedOutputSize)
	inputPos, outputPos := 0, 0
	var currentFlag byte = 0x00
	bitCount := 0
	flagPos := outputPos
	startTime := time.Now()
	lastProgressTime := startTime
	lastProgressPos := 0
	totalInputLen := float64(len(inputData))

	if outputPos >= len(outputBuffer) {
		return nil, errBufferTooSmall
	}
	outputBuffer[flagPos] = 0x00
	outputPos++
	fmt.Println("Starting LZSS compression...")

	for inputPos < len(inputData) {
		currentTime := time.Now()
		if currentTime.Sub(lastProgressTime).Seconds() >= 0.5 || inputPos == len(inputData)-1 || (totalInputLen > 0 && float64(inputPos-lastProgressPos)/totalInputLen > 0.05) {
			progress := float64(inputPos) / totalInputLen * 100
			fmt.Printf("\rCompressing... %.1f%% (%d/%d)", progress, inputPos, len(inputData))
			lastProgressTime = currentTime
			lastProgressPos = inputPos
		}
		if outputPos >= len(outputBuffer) {
			return nil, errBufferTooSmall
		}
		currentWindowSize := inputPos
		if currentWindowSize > maxWindowSize {
			currentWindowSize = maxWindowSize
		}
		currentMaxMatchLen := len(inputData) - inputPos
		if currentMaxMatchLen > maxEncodedLength {
			currentMaxMatchLen = maxEncodedLength
		}

		matchOffset, matchLength, matchFound := findMatch(inputData, inputPos, currentWindowSize, currentMaxMatchLen)

		if !matchFound {
			bitCount++
			if bitCount == 8 {
				outputBuffer[flagPos] = currentFlag
				currentFlag = 0x00
				bitCount = 0
				flagPos = outputPos
				if outputPos >= len(outputBuffer) {
					return nil, errBufferTooSmall
				}
				outputBuffer[flagPos] = 0x00
				outputPos++
			}
			if outputPos >= len(outputBuffer) {
				return nil, errBufferTooSmall
			}
			outputBuffer[outputPos] = inputData[inputPos]
			inputPos++
			outputPos++
		} else {
			currentFlag |= (1 << (7 - bitCount))
			bitCount++
			if bitCount == 8 {
				outputBuffer[flagPos] = currentFlag
				currentFlag = 0x00
				bitCount = 0
				flagPos = outputPos
				if outputPos >= len(outputBuffer) {
					return nil, errBufferTooSmall
				}
				outputBuffer[flagPos] = 0x00
				outputPos++
			}
			if outputPos+1 >= len(outputBuffer) {
				return nil, errBufferTooSmall
			}
			encodedPairVal := uint16(((matchOffset - 1) << 4) | ((matchLength - 3) & 0x0F))
			outputBuffer[outputPos] = byte(encodedPairVal & 0xFF)
			outputPos++
			outputBuffer[outputPos] = byte((encodedPairVal >> 8) & 0xFF)
			outputPos++
			inputPos += matchLength
		}
	}
	outputBuffer[flagPos] = currentFlag
	fmt.Printf("\rCompressing... 100.0%% (%d/%d) Done. Time: %s\n", len(inputData), len(inputData), time.Since(startTime))
	return outputBuffer[:outputPos], nil
}

// Compression and Padding
func compressAndPadToPCMP(inputData []byte) ([]byte, error) {
	totalUncompressedSize := uint32(len(inputData))
	fmt.Printf("Input size: %d bytes\n", totalUncompressedSize)

	compressedStream, err := coreCompressV1(inputData)
	if err != nil {
		return nil, fmt.Errorf("core LZSS compression failed: %w", err)
	}
	totalCompressedStreamSize := uint32(len(compressedStream))
	fmt.Printf("Compressed LZSS stream size: %d bytes\n", totalCompressedStreamSize)

	pcmpHeader, err := writePCMPHeader(totalUncompressedSize, totalCompressedStreamSize)
	if err != nil {
		return nil, fmt.Errorf("creating PCMP header failed: %w", err)
	}

	finalOutputData := append(pcmpHeader, compressedStream...)
	fmt.Printf("PCMP data size before padding: %d bytes (0x%X)\n", len(finalOutputData), len(finalOutputData))

	currentSize := len(finalOutputData)
	remainder := currentSize % fileAlignment
	var paddingNeeded int = 0

	if remainder != 0 {
		paddingNeeded = fileAlignment - remainder
		fmt.Printf("Padding needed to reach %d byte alignment: %d bytes\n", fileAlignment, paddingNeeded)
	} else {
		fmt.Printf("Data size is already aligned to %d bytes.\n", fileAlignment)
	}

	if paddingNeeded > 0 {
		padding := make([]byte, paddingNeeded) // Creates a slice initialized to zeros
		finalOutputData = append(finalOutputData, padding...)
	}
	fmt.Printf("Final padded file size: %d bytes (0x%X)\n", len(finalOutputData), len(finalOutputData))

	return finalOutputData, nil
}

// --- Main EXE ---
func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <inputfile> [outputfile.pcmp]\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "If outputfile is not specified, it will be <inputfile_basename>.pcmp\n")
		os.Exit(1)
	}
	inputFile := os.Args[1]
	outputFile := ""
	if len(os.Args) == 3 {
		outputFile = os.Args[2]
	} else {
		outputFile = strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + ".pcmp"
	}

	inputData, err := ioutil.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input file %s: %v\n", inputFile, err)
		os.Exit(1)
	}
	if len(inputData) == 0 {
		fmt.Fprintf(os.Stderr, "Input file %s is empty. Creating minimal padded PCMP file.\n", inputFile)
		header, _ := writePCMPHeader(0, 0)
		paddedHeader := make([]byte, fileAlignment)
		copy(paddedHeader, header)
		err = ioutil.WriteFile(outputFile, paddedHeader, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing empty PCMP file %s: %v\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Printf("Empty file processed to %s\n", outputFile)
		os.Exit(0)
	}

	fmt.Printf("Compressing %s to %s...\n", inputFile, outputFile)
	compressedPaddedData, err := compressAndPadToPCMP(inputData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during PCMP compression/padding: %v\n", err)
		os.Exit(1)
	}

	err = ioutil.WriteFile(outputFile, compressedPaddedData, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file %s: %v\n", outputFile, err)
		os.Exit(1)
	}

	fmt.Printf("Successfully compressed and padded %s to %s (%d bytes -> %d bytes)\n", inputFile, outputFile, len(inputData), len(compressedPaddedData))
}
