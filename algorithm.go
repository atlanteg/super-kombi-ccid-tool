package main

import "sort"

// GroupResult holds original and modified bytes for one CC-ID group.
type GroupResult struct {
	GroupNum        int
	OriginalBytes   []byte
	ModifiedBytes   []byte
	ModifiedIndices []int // byte indices that were changed
}

// getGroupNumber returns the 1-based group number for a CC-ID.
// Each group covers 64 CC-IDs (8 bytes × 8 bits).
// Reproduced from CCID-Calculator.exe bytecode: cc_id // 64 + 1
func getGroupNumber(ccID int) int {
	return ccID/64 + 1
}

// calculateMask applies CC-ID activation to the initial byte states.
// For each selected CC-ID it CLEARS the corresponding bit (BMW convention:
// bit=0 → CC-ID active/visible, bit=1 → CC-ID masked/suppressed).
// initialStates: map[groupNum] → 8-byte slice (from CAFD or FF defaults)
func calculateMask(initialStates map[int][]byte, selectedIDs []int) []*GroupResult {
	resultMap := make(map[int]*GroupResult)

	for _, ccID := range selectedIDs {
		groupNum := getGroupNumber(ccID)

		if _, exists := resultMap[groupNum]; !exists {
			original := make([]byte, 8)
			if init, ok := initialStates[groupNum]; ok && len(init) == 8 {
				copy(original, init)
			} else {
				for i := range original {
					original[i] = 0xFF
				}
			}
			modified := make([]byte, 8)
			copy(modified, original)
			resultMap[groupNum] = &GroupResult{
				GroupNum:      groupNum,
				OriginalBytes: original,
				ModifiedBytes: modified,
			}
		}

		gr := resultMap[groupNum]
		bitPos := ccID % 64
		byteIdx := bitPos / 8
		bitIdx := uint(bitPos % 8)

		// Clear the bit: &^= is Go's bit-clear (AND NOT) operator
		gr.ModifiedBytes[byteIdx] &^= byte(1) << bitIdx

		// Track unique modified byte indices
		found := false
		for _, idx := range gr.ModifiedIndices {
			if idx == byteIdx {
				found = true
				break
			}
		}
		if !found {
			gr.ModifiedIndices = append(gr.ModifiedIndices, byteIdx)
		}
	}

	results := make([]*GroupResult, 0, len(resultMap))
	for _, gr := range resultMap {
		sort.Ints(gr.ModifiedIndices)
		results = append(results, gr)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].GroupNum < results[j].GroupNum
	})
	return results
}
