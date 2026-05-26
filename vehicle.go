package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sort"
	"time"
)

// LiveCCID is a CC-ID currently active / stored in the vehicle's instrument cluster.
type LiveCCID struct {
	ID          int
	Description string
	// FlagBytes holds the two status bytes returned by the cluster (for reference).
	FlagBytes [2]byte
}

// ReadVehicleCCIDs connects to the BMW EDIABAS/AIFC TCP interface at host:6801,
// sends a UDS ReadDataByIdentifier (22 D1 0B) to the Kombi (ECU 0x60), and
// returns the list of stored CC-IDs with descriptions from the embedded database.
//
// Protocol observed in traffic capture (ISTA ↔ VCI, port 6801):
//
//	Frame:   [4B big-endian payload-length][2B type][payload]
//	Type:    0x0001 = data frame, 0x0002 = ACK (echoes request)
//	Request: payload = [src=0xF4][dst=0x60][UDS service 0x22][DID_HI 0xD1][DID_LO 0x0B]
//	Response payload starts: [src=0x60][dst=0xF4][0x62][0xD1][0x0B] then 4-byte records
//	Record:  [CC_ID_HI][CC_ID_LO][FLAG1][FLAG2] — zeros-padded tail = end of list
func ReadVehicleCCIDs(host string) ([]LiveCCID, error) {
	addr := fmt.Sprintf("%s:6801", host)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to VCI at %s: %w", addr, err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return nil, err
	}

	// UDS 22 D1 0B: ReadDataByIdentifier DID=D10B, from addr 0xF4 to Kombi 0x60.
	request := []byte{0xF4, 0x60, 0x22, 0xD1, 0x0B}
	if err := ediabas_write(conn, request); err != nil {
		return nil, fmt.Errorf("send error: %w", err)
	}

	// Expect an ACK frame (type=0002) followed by the data frame (type=0001).
	// We read up to 3 frames to tolerate out-of-order or extra messages.
	for attempt := 0; attempt < 3; attempt++ {
		msgType, payload, err := ediabas_read(conn)
		if err != nil {
			return nil, fmt.Errorf("read frame: %w", err)
		}
		if msgType == 0x0001 {
			return parseCCIDResponse(payload)
		}
		// msgType 0x0002 = ACK echo — loop back and wait for data.
	}
	return nil, fmt.Errorf("no data response received from VCI")
}

// ediabas_write sends one EDIABAS data frame (type=0x0001) over conn.
func ediabas_write(w io.Writer, payload []byte) error {
	buf := make([]byte, 6+len(payload))
	binary.BigEndian.PutUint32(buf[0:], uint32(len(payload)))
	binary.BigEndian.PutUint16(buf[4:], 0x0001)
	copy(buf[6:], payload)
	_, err := w.Write(buf)
	return err
}

// ediabas_read reads one EDIABAS frame from conn and returns its type and payload.
func ediabas_read(r io.Reader) (msgType uint16, payload []byte, err error) {
	hdr := make([]byte, 6)
	if _, err = io.ReadFull(r, hdr); err != nil {
		return
	}
	length := binary.BigEndian.Uint32(hdr[0:4])
	msgType = binary.BigEndian.Uint16(hdr[4:6])
	if length > 0 {
		payload = make([]byte, length)
		_, err = io.ReadFull(r, payload)
	}
	return
}

// parseCCIDResponse decodes the UDS 62 D1 0B response payload.
// payload[0..1] = EDIABAS src/dst addresses
// payload[2..4] = UDS 0x62 0xD1 0x0B
// payload[5..]  = 4-byte records: [CC_ID_HI][CC_ID_LO][FLAG1][FLAG2]
func parseCCIDResponse(payload []byte) ([]LiveCCID, error) {
	const minLen = 5
	if len(payload) < minLen {
		return nil, fmt.Errorf("response too short: %d bytes", len(payload))
	}
	// Check for UDS negative response (service 0x7F)
	if payload[2] == 0x7F {
		nrc := byte(0)
		if len(payload) > 4 {
			nrc = payload[4]
		}
		return nil, fmt.Errorf("UDS negative response (NRC=0x%02X)", nrc)
	}
	if payload[2] != 0x62 || payload[3] != 0xD1 || payload[4] != 0x0B {
		return nil, fmt.Errorf("unexpected UDS response: %02X %02X %02X",
			payload[2], payload[3], payload[4])
	}

	data := payload[minLen:]
	descs := loadDescriptions()

	var results []LiveCCID
	for i := 0; i+3 < len(data); i += 4 {
		hi, lo := data[i], data[i+1]
		flag1, flag2 := data[i+2], data[i+3]
		if hi == 0 && lo == 0 && flag1 == 0 && flag2 == 0 {
			break // zero-padded tail = end of stored CC-IDs
		}
		ccid := int(hi)<<8 | int(lo)
		desc := descs[ccid]
		if desc == "" {
			desc = fmt.Sprintf("CC-ID %d (not in database)", ccid)
		}
		results = append(results, LiveCCID{
			ID:          ccid,
			Description: desc,
			FlagBytes:   [2]byte{flag1, flag2},
		})
	}

	// Sort by CC-ID for deterministic display
	sort.Slice(results, func(i, j int) bool { return results[i].ID < results[j].ID })
	return results, nil
}
