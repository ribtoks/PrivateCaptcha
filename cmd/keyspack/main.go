package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

type hardcodedKey struct {
	KeyID   int    `json:"KeyID"`
	KeyData string `json:"KeyData"`
}

func handleWrite(filePath string, useBase64 bool) {
	jsonFile, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	var keys []hardcodedKey
	if err := json.Unmarshal(jsonFile, &keys); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	var buf bytes.Buffer

	for _, k := range keys {
		decodedData, err := base64.StdEncoding.DecodeString(k.KeyData)
		if err != nil {
			log.Fatalf("Failed to decode base64 data for KeyID %d: %v", k.KeyID, err)
		}
		if len(decodedData) > 256 {
			log.Fatalf("Decoded data size does not fit into 1 byte")
		}

		keyID := uint16(k.KeyID)
		dataLen := uint8(len(decodedData))

		if err := binary.Write(&buf, binary.LittleEndian, keyID); err != nil {
			log.Fatalf("Failed to write KeyID: %v", err)
		}
		if err := binary.Write(&buf, binary.LittleEndian, dataLen); err != nil {
			log.Fatalf("Failed to write data length: %v", err)
		}
		if _, err := buf.Write(decodedData); err != nil {
			log.Fatalf("Failed to write key data: %v", err)
		}
	}

	if useBase64 {
		encodedString := base64.StdEncoding.EncodeToString(buf.Bytes())
		if _, err := os.Stdout.WriteString(encodedString); err != nil {
			log.Fatalf("Failed to write to stdout: %v", err)
		}
	} else {
		if _, err := os.Stdout.Write(buf.Bytes()); err != nil {
			log.Fatalf("Failed to write to stdout: %v", err)
		}
	}
}

func handleRead(useBase64 bool) {
	var inputReader io.Reader = os.Stdin
	if useBase64 {
		inputReader = base64.NewDecoder(base64.StdEncoding, os.Stdin)
	}

	binaryData, err := io.ReadAll(inputReader)
	if err != nil {
		log.Fatalf("Failed to read from stdin: %v", err)
	}

	reader := bytes.NewReader(binaryData)
	var keys []hardcodedKey

	for reader.Len() > 0 {
		var keyID uint16
		if err := binary.Read(reader, binary.LittleEndian, &keyID); err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalf("Failed to read KeyID: %v", err)
		}

		var dataLen uint8
		if err := binary.Read(reader, binary.LittleEndian, &dataLen); err != nil {
			log.Fatalf("Failed to read data length: %v", err)
		}

		if reader.Len() < int(dataLen) {
			log.Fatalf("Incomplete data: expected %d bytes, but only %d remain", dataLen, reader.Len())
		}

		data := make([]byte, dataLen)
		if _, err := io.ReadFull(reader, data); err != nil {
			log.Fatalf("Failed to read key data: %v", err)
		}

		encodedData := base64.StdEncoding.EncodeToString(data)

		keys = append(keys, hardcodedKey{
			KeyID:   int(keyID),
			KeyData: encodedData,
		})
	}

	jsonData, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	fmt.Print(string(jsonData))
}

func main() {
	log.SetOutput(os.Stderr)
	mode := flag.String("mode", "", "Mode of operation: read or write")
	filePath := flag.String("file", "", "Path to the JSON file (for write mode)")
	useBase64 := flag.Bool("base64", false, "Use base64 encoding for input/output")
	flag.Parse()

	switch *mode {
	case "write":
		if *filePath == "" {
			log.Fatal("The -file argument is required for write mode")
		}
		handleWrite(*filePath, *useBase64)
	case "read":
		if *filePath != "" {
			log.Fatal("The -file argument is not used for read mode")
		}
		handleRead(*useBase64)
	default:
		log.Fatal("Invalid mode. Please use 'read' or 'write'")
	}
}
