// Code generated by "stringer -type=ID --linecomment"; DO NOT EDIT.

package filesystem

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[IPFS-1]
	_ = x[IPNS-2]
	_ = x[Files-3]
	_ = x[PinFS-4]
	_ = x[KeyFS-5]
}

const _ID_name = "IPFSIPNSfilePinFSKeyFS"

var _ID_index = [...]uint8{0, 4, 8, 12, 17, 22}

func (i ID) String() string {
	i -= 1
	if i >= ID(len(_ID_index)-1) {
		return "ID(" + strconv.FormatInt(int64(i+1), 10) + ")"
	}
	return _ID_name[_ID_index[i]:_ID_index[i+1]]
}
