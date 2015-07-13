// Copyright 2014 Jamie Hall. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package frames

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/SlyMarbo/spdy/common"
)

type HEADERS struct {
	Flags     common.Flags
	StreamID  common.StreamID
	Header    http.Header
	rawHeader []byte
}

func (frame *HEADERS) Compress(com common.Compressor) error {
	if frame.rawHeader != nil {
		return nil
	}

	data, err := com.Compress(frame.Header)
	if err != nil {
		return err
	}

	frame.rawHeader = data
	return nil
}

func (frame *HEADERS) Decompress(decom common.Decompressor) error {
	if frame.Header != nil {
		return nil
	}

	header, err := decom.Decompress(frame.rawHeader)
	if err != nil {
		return err
	}

	frame.Header = header
	frame.rawHeader = nil
	return nil
}

func (frame *HEADERS) Name() string {
	return "HEADERS"
}

func (frame *HEADERS) ReadFrom(reader io.Reader) (int64, error) {
	data, err := common.ReadExactly(reader, 12)
	if err != nil {
		return 0, err
	}

	err = controlFrameCommonProcessing(data[:5], _HEADERS, common.FLAG_FIN)
	if err != nil {
		return 12, err
	}

	// Get and check length.
	length := int(common.BytesToUint24(data[5:8]))
	if length < 4 {
		return 12, common.IncorrectDataLength(length, 4)
	} else if length > common.MAX_FRAME_SIZE-8 {
		return 12, common.FrameTooLarge
	}

	// Read in data.
	header, err := common.ReadExactly(reader, length-4)
	if err != nil {
		return 12, err
	}

	frame.Flags = common.Flags(data[4])
	frame.StreamID = common.StreamID(common.BytesToUint32(data[8:12]))
	frame.rawHeader = header

	if !frame.StreamID.Valid() {
		return int64(length) + 8, common.StreamIdTooLarge
	}
	if frame.StreamID.Zero() {
		return int64(length) + 8, common.StreamIdIsZero
	}

	return int64(length) + 8, nil
}

func (frame *HEADERS) String() string {
	buf := new(bytes.Buffer)

	flags := ""
	if frame.Flags.FIN() {
		flags += " common.FLAG_FIN"
	}
	if flags == "" {
		flags = "[NONE]"
	} else {
		flags = flags[1:]
	}

	buf.WriteString("HEADERS {\n\t")
	buf.WriteString(fmt.Sprintf("Version:              3\n\t"))
	buf.WriteString(fmt.Sprintf("Flags:                %s\n\t", flags))
	buf.WriteString(fmt.Sprintf("Stream ID:            %d\n\t", frame.StreamID))
	buf.WriteString(fmt.Sprintf("Header:               %#v\n}\n", frame.Header))

	return buf.String()
}

func (frame *HEADERS) WriteTo(writer io.Writer) (int64, error) {
	if frame.rawHeader == nil {
		return 0, errors.New("Error: Headers not written.")
	}
	if !frame.StreamID.Valid() {
		return 0, common.StreamIdTooLarge
	}
	if frame.StreamID.Zero() {
		return 0, common.StreamIdIsZero
	}

	header := frame.rawHeader
	length := 4 + len(header)
	out := make([]byte, 12)

	out[0] = 128                  // Control bit and Version
	out[1] = 3                    // Version
	out[2] = 0                    // Type
	out[3] = 8                    // Type
	out[4] = byte(frame.Flags)    // Flags
	out[5] = byte(length >> 16)   // Length
	out[6] = byte(length >> 8)    // Length
	out[7] = byte(length)         // Length
	out[8] = frame.StreamID.B1()  // Stream ID
	out[9] = frame.StreamID.B2()  // Stream ID
	out[10] = frame.StreamID.B3() // Stream ID
	out[11] = frame.StreamID.B4() // Stream ID

	err := common.WriteExactly(writer, out)
	if err != nil {
		return 0, err
	}

	err = common.WriteExactly(writer, header)
	if err != nil {
		return 12, err
	}

	return int64(length + 8), nil
}
