package idmap

import (
	"fmt"
	"strconv"
	"strings"
)

// Deterministic, reversible encoding of YouTrack string IDs into Jira-style
// numeric int64 IDs. No file I/O, no state — pure functions.
//
// YouTrack IDs come in two formats:
//   - Base entity:    "<typeId>-<seqId>"              e.g. "0-4", "69-774"
//   - Activity stream: "<typeId>-<seqId>.<catId>-<eventOffset>" e.g. "0-0.9-60"
//
// Encoding uses a tagged variable layout. The flag bit (b62) selects the mode:
//
// Mode A: Base Entity (Flag = 0)
//
//	┌─────┬──────────┬──────────────────────────────────────────────────────────┐
//	│Flag │ TypeId   │                       SeqId                             │
//	│1 bit│ 8 bits   │                      54 bits                            │
//	│b62  │ b61..b54 │                     b53..b0                             │
//	└─────┴──────────┴──────────────────────────────────────────────────────────┘
//
// Mode B: Activity Stream (Flag = 1)
//
//	┌─────┬──────────┬────────────────────────┬────────┬─────────────────────────┐
//	│Flag │ TypeId   │        SeqId           │ CatId  │      EventOffset        │
//	│1 bit│ 7 bits   │       24 bits          │ 6 bits │       25 bits           │
//	│b62  │ b61..b55 │      b54..b31          │b30..b25│       b24..b0           │
//	└─────┴──────────┴────────────────────────┴────────┴─────────────────────────┘

const (
	flagBit = 62

	// Mode A: Base entity (flag = 0)
	baseTypeShift = 54
	baseTypeBits  = 8
	baseTypeMask  = (1 << baseTypeBits) - 1 // 0xFF — max TypeId 255
	baseSeqBits   = 54
	baseSeqMask   = (1 << baseSeqBits) - 1 // 0x3FFFFFFFFFFFFF — max SeqId 18,014,398,509,481,983

	// Mode B: Activity stream (flag = 1)
	actTypeShift = 55
	actTypeBits  = 7
	actTypeMask  = (1 << actTypeBits) - 1 // 0x7F — max TypeId 127
	actSeqShift  = 31
	actSeqBits   = 24
	actSeqMask   = (1 << actSeqBits) - 1 // 0xFFFFFF — max SeqId 16,777,215
	actCatShift  = 25
	actCatBits   = 6
	actCatMask   = (1 << actCatBits) - 1 // 0x3F — max CatId 63
	actEventBits = 25
	actEventMask = (1 << actEventBits) - 1 // 0x1FFFFFF — max EventOffset 33,554,431
)

// Encode converts a YouTrack string ID (e.g. "0-4", "0-0.9-60") into a
// deterministic int64 by parsing the ID into numeric fields and packing them
// into a 63-bit bitfield. Returns an error for malformed IDs or field overflow.
func Encode(youtrackID string) (int64, error) {
	typeId, seqId, catId, eventOffset, isActivity, err := parseYouTrackID(youtrackID)
	if err != nil {
		return 0, err
	}

	if isActivity {
		// Mode B: Activity stream (flag = 1)
		if typeId > actTypeMask {
			return 0, fmt.Errorf("idmap: typeId %d exceeds %d-bit capacity (max %d) in YouTrack ID %q", typeId, actTypeBits, int64(actTypeMask), youtrackID)
		}
		if seqId > actSeqMask {
			return 0, fmt.Errorf("idmap: seqId %d exceeds %d-bit capacity (max %d) in YouTrack ID %q", seqId, actSeqBits, int64(actSeqMask), youtrackID)
		}
		if catId > actCatMask {
			return 0, fmt.Errorf("idmap: catId %d exceeds %d-bit capacity (max %d) in YouTrack ID %q", catId, actCatBits, int64(actCatMask), youtrackID)
		}
		if eventOffset > actEventMask {
			return 0, fmt.Errorf("idmap: eventOffset %d exceeds %d-bit capacity (max %d) in YouTrack ID %q", eventOffset, actEventBits, int64(actEventMask), youtrackID)
		}
		return (1 << flagBit) | (typeId << actTypeShift) | (seqId << actSeqShift) | (catId << actCatShift) | eventOffset, nil
	}

	// Mode A: Base entity (flag = 0)
	if typeId > baseTypeMask {
		return 0, fmt.Errorf("idmap: typeId %d exceeds %d-bit capacity (max %d) in YouTrack ID %q", typeId, baseTypeBits, int64(baseTypeMask), youtrackID)
	}
	if seqId > baseSeqMask {
		return 0, fmt.Errorf("idmap: seqId %d exceeds %d-bit capacity (max %d) in YouTrack ID %q", seqId, baseSeqBits, int64(baseSeqMask), youtrackID)
	}
	return (typeId << baseTypeShift) | seqId, nil
}

// Decode reverses Encode: given a packed int64 it extracts the bitfield
// components and reconstructs the original YouTrack string ID.
// Returns ("", false) for negative input.
func Decode(numericID int64) (string, bool) {
	if numericID < 0 {
		return "", false
	}

	flag := (numericID >> flagBit) & 0x1

	if flag == 0 {
		// Mode A: Base entity
		typeId := (numericID >> baseTypeShift) & int64(baseTypeMask)
		seqId := numericID & int64(baseSeqMask)
		return fmt.Sprintf("%d-%d", typeId, seqId), true
	}

	// Mode B: Activity stream
	typeId := (numericID >> actTypeShift) & int64(actTypeMask)
	seqId := (numericID >> actSeqShift) & int64(actSeqMask)
	catId := (numericID >> actCatShift) & int64(actCatMask)
	eventOffset := numericID & int64(actEventMask)
	return fmt.Sprintf("%d-%d.%d-%d", typeId, seqId, catId, eventOffset), true
}

// FormatID returns a numeric ID as a decimal string.
func FormatID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// parseYouTrackID parses a YouTrack ID into its constituent numeric fields.
//
// Base entity: "2-105"     → typeId=2, seqId=105, catId=0, eventOffset=0, isActivity=false
// Activity:    "0-0.9-60"  → typeId=0, seqId=0, catId=9, eventOffset=60, isActivity=true
func parseYouTrackID(id string) (typeId, seqId, catId, eventOffset int64, isActivity bool, err error) {
	if !strings.Contains(id, "-") {
		return 0, 0, 0, 0, false, fmt.Errorf("idmap: malformed YouTrack ID %q (no hyphen)", id)
	}

	dotIdx := strings.Index(id, ".")
	if dotIdx < 0 {
		// Base entity: "<typeId>-<seqId>"
		parts := strings.SplitN(id, "-", 3)
		if len(parts) != 2 {
			return 0, 0, 0, 0, false, fmt.Errorf("idmap: malformed YouTrack ID %q (expected typeId-seqId)", id)
		}
		typeId, err = parseNonNeg(parts[0], id)
		if err != nil {
			return 0, 0, 0, 0, false, err
		}
		seqId, err = parseNonNeg(parts[1], id)
		if err != nil {
			return 0, 0, 0, 0, false, err
		}
		return typeId, seqId, 0, 0, false, nil
	}

	// Activity stream: "<typeId>-<seqId>.<catId>-<eventOffset>"
	basePart := id[:dotIdx]
	actPart := id[dotIdx+1:]

	baseParts := strings.SplitN(basePart, "-", 3)
	if len(baseParts) != 2 {
		return 0, 0, 0, 0, false, fmt.Errorf("idmap: malformed YouTrack ID %q (expected typeId-seqId before dot)", id)
	}
	typeId, err = parseNonNeg(baseParts[0], id)
	if err != nil {
		return 0, 0, 0, 0, false, err
	}
	seqId, err = parseNonNeg(baseParts[1], id)
	if err != nil {
		return 0, 0, 0, 0, false, err
	}

	actParts := strings.SplitN(actPart, "-", 3)
	if len(actParts) != 2 {
		return 0, 0, 0, 0, false, fmt.Errorf("idmap: malformed activity suffix in YouTrack ID %q", id)
	}
	catId, err = parseNonNeg(actParts[0], id)
	if err != nil {
		return 0, 0, 0, 0, false, err
	}
	eventOffset, err = parseNonNeg(actParts[1], id)
	if err != nil {
		return 0, 0, 0, 0, false, err
	}

	return typeId, seqId, catId, eventOffset, true, nil
}

// parseNonNeg parses s as a non-negative int64, returning an idmap:-prefixed error on failure.
func parseNonNeg(s string, origID string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("idmap: non-numeric component in YouTrack ID %q: %v", origID, err)
	}
	if v < 0 {
		return 0, fmt.Errorf("idmap: negative value in YouTrack ID %q", origID)
	}
	return v, nil
}
