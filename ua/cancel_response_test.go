// Copyright 2018-2020 opcua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ua

import (
	"testing"
	"time"
)

func TestCancelResponse(t *testing.T) {
	cases := []CodecTestCase{
		{
			Name: "normal",
			Struct: &CancelResponse{
				ResponseHeader: &ResponseHeader{
					Timestamp:          time.Date(2018, time.August, 10, 23, 0, 0, 0, time.UTC),
					RequestHandle:      1,
					ServiceDiagnostics: &DiagnosticInfo{},
					StringTable:        []string{},
					AdditionalHeader:   NewExtensionObject(nil),
				},
				CancelCount: 1,
			},
			Bytes: []byte{
				// Timestamp
				0x00, 0x98, 0x67, 0xdd, 0xfd, 0x30, 0xd4, 0x01,
				// RequestHandle
				0x01, 0x00, 0x00, 0x00,
				// ServiceResult
				0x00, 0x00, 0x00, 0x00,
				// ServiceDiagnostics
				0x00,
				// StringTable
				0x00, 0x00, 0x00, 0x00,
				// AdditionalHeader
				0x00, 0x00, 0x00,
				// RequestHandle
				0x01, 0x00, 0x00, 0x00,
			},
		},
	}
	RunCodecTest(t, cases)
}
