## Panzer Dragoon Orta: Compressed Data & TXB Texture Tools.

command-line tools for handling TXB texture formats used in Panzer Dragoon Orta (Original Xbox), with a particular focus on its PCMP (a variant of LZSS) compression scheme.

This project currently includes the following main command-line tools:

1.  **PCMP Decompressor:**
    *   Decompresses the game's original PCMP-compressed TXB files to raw TXB texture data.
    *   Usage: `pcmp_decompressor.exe compressed_input.txb decompressed_output.txb`

2.  **PCMP Compressor:**
    *   Compresses raw TXB texture data (e.g., modified textures) back into the game's recognizable PCMP format.
    *   Implements a game-compatible specific LZSS variant (longest match first; if lengths are equal, prioritizes the match with the largest/oldest offset).
    *   Pads the output file with null bytes to ensure compliance with Xbox DVD storage alignment rules.
    *   Usage:
        ```pcmp_compressor.exe uncompressed_input.txb compressed_output.txb```

3.  **PNG to TXB (DXT3) Converter:**
    *   Converts PNG images to raw TXB data in DXT3 format.
    *   Usage: `png_to_txb_dxt3.exe input.png original_ref.txb output_new.txb`
    *   Dependency: Requires `texconv.exe` (Microsoft DirectX Texture Converter) to be in the same directory as this tool or in the system's PATH.

## TXB Texture Format Introduction (Preliminary Observations)

TXB is a simple texture container format used by Orta. My understanding of it is still partial:

*   **Known Pixel Formats within TXB:**
    *   **DXT3 (BC2):** A common block compression format used for textures with an explicit Alpha channel. Microsoft's open-source `texconv.exe` can be used to process this format; it can convert PNGs into DDS texture format containing DXT3 data streams, which can then be transferred into a TXB file.
    *   **RGBA8888:** An uncompressed 32-bit color depth format. In this game, texture data in this format undergoes **Morton (Z-order curve) Swizzling**, as seen, for example, in the title screen.

*   **Mipmaps:** At least some RGBA8888 TXB files include Mipmaps (e.g., a 512x512 main texture + a 256x256 Mipmap). Whether DXT3 TXB files commonly include Mipmaps is yet to be confirmed, but `texconv` generates them by default; we use the `-m 1` parameter to avoid this.

## Build Instructions

1.  Install the Go environment.
2.  Then, :
    ```bash
    go build tool_name.go 
    ```
    

## Acknowledgements

*   **Rabatini:** Provided the original Python decompression code.
*   **DKDave:** Pointed out the pixel formats of the textures.
