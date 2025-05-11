package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	txbHeaderSize = 0x20
	ddsHeaderSize = 0x90
)

func runTexconv(texconvPath, inputPNGPath, tempDir string) (string, error) {
	if _, err := os.Stat(texconvPath); os.IsNotExist(err) {
		return "", fmt.Errorf("texconv.exe not found at: %s", texconvPath)
	}
	if _, err := os.Stat(inputPNGPath); os.IsNotExist(err) {
		return "", fmt.Errorf("input PNG file not found: %s", inputPNGPath)
	}

	inputBasename := strings.TrimSuffix(filepath.Base(inputPNGPath), filepath.Ext(inputPNGPath))
	outputDDSPath := filepath.Join(tempDir, inputBasename+".dds")

	cmd := exec.Command(texconvPath,
		"-f", "BC2_UNORM",
		"-m", "1",
		"-nologo",
		"-y",
		"-o", tempDir,
		inputPNGPath,
	)

	fmt.Printf("Executing texconv: %s\n", strings.Join(cmd.Args, " "))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("texconv execution failed: %v\nOutput:\n%s", err, string(output))
	}
	fmt.Printf("Texconv output:\n%s\n", string(output))

	if _, err := os.Stat(outputDDSPath); os.IsNotExist(err) {
		return "", fmt.Errorf("texconv ran but expected output DDS file not found: %s. Check texconv output.", outputDDSPath)
	}

	return outputDDSPath, nil
}

func createNewTXB(originalTXBPath, ddsDataPath, outputNewTXBPath string) error {
	originalTXBFile, err := os.Open(originalTXBPath)
	if err != nil {
		return fmt.Errorf("reading original TXB file '%s': %w", originalTXBPath, err)
	}
	defer originalTXBFile.Close()

	originalHeader := make([]byte, txbHeaderSize)
	n, err := originalTXBFile.Read(originalHeader)
	if err != nil || n < txbHeaderSize {
		return fmt.Errorf("reading original TXB header (need %d bytes, got %d): %w", txbHeaderSize, n, err)
	}
	fmt.Printf("Read original TXB header (%d bytes) from %s\n", len(originalHeader), originalTXBPath)

	ddsFile, err := os.Open(ddsDataPath)
	if err != nil {
		return fmt.Errorf("reading DDS data file '%s': %w", ddsDataPath, err)
	}
	defer ddsFile.Close()

	ddsStat, err := ddsFile.Stat()
	if err != nil {
		return fmt.Errorf("getting DDS file stats: %w", err)
	}
	if ddsStat.Size() <= ddsHeaderSize {
		return fmt.Errorf("DDS file '%s' is too small (size %d, expected > %d)", ddsDataPath, ddsStat.Size(), ddsHeaderSize)
	}

	_, err = ddsFile.Seek(ddsHeaderSize, io.SeekStart)
	if err != nil {
		return fmt.Errorf("seeking in DDS file: %w", err)
	}

	dxt3Data, err := ioutil.ReadAll(ddsFile)
	if err != nil {
		return fmt.Errorf("reading DXT3 data from DDS: %w", err)
	}
	fmt.Printf("Read DXT3 data (%d bytes) from %s (after skipping %d byte DDS header)\n", len(dxt3Data), ddsDataPath, ddsHeaderSize)

	fmt.Println("Using original TXB header as-is (no size fields updated yet).")

	newTXBData := append(originalHeader, dxt3Data...)
	fmt.Printf("New TXB data created (header %d bytes + DXT3 %d bytes = total %d bytes)\n", len(originalHeader), len(dxt3Data), len(newTXBData))

	err = ioutil.WriteFile(outputNewTXBPath, newTXBData, 0644)
	if err != nil {
		return fmt.Errorf("writing new TXB file '%s': %w", outputNewTXBPath, err)
	}
	fmt.Printf("Successfully created new TXB file: %s\n", outputNewTXBPath)
	return nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input_png_path> <original_txb_path> <output_new_txb_path>\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "Example: %s menu_cn.png menu_original.txb menu_new.txb\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	inputPNGPath := os.Args[1]
	originalTXBPath := os.Args[2]
	outputNewTXBPath := os.Args[3]

	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}
	texconvPath := filepath.Join(filepath.Dir(exePath), "texconv.exe")
	if _, err := os.Stat(texconvPath); os.IsNotExist(err) {
		cwd, _ := os.Getwd()
		texconvPath = filepath.Join(cwd, "texconv.exe")
		if _, err := os.Stat(texconvPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: texconv.exe not found next to the executable or in the current working directory (%s or %s)\n", filepath.Join(filepath.Dir(exePath), "texconv.exe"), filepath.Join(cwd, "texconv.exe"))
			os.Exit(1)
		}
	}

	tempDir, err := ioutil.TempDir("", "png2txb-tempdds-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temporary directory: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)
	fmt.Printf("Temporary directory for DDS: %s\n", tempDir)

	fmt.Println("\n--- Step 1: Converting PNG to DDS ---")
	ddsPath, err := runTexconv(texconvPath, inputPNGPath, tempDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during PNG to DDS conversion: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("DDS file created at: %s\n", ddsPath)

	fmt.Println("\n--- Step 2: Creating new TXB file ---")
	err = createNewTXB(originalTXBPath, ddsPath, outputNewTXBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating new TXB file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nProcess completed successfully!")
}
