// Code generated by "stringer -type=Namespace -trimprefix=Namespace -linecomment"; DO NOT EDIT.

package mountinter

import "strconv"

func _() {
	// An "invalid array index" compiler error signifies that the constant values have changed.
	// Re-run the stringer command to generate them again.
	var x [1]struct{}
	_ = x[NamespaceNone-0]
	_ = x[NamespaceCore-1]
	_ = x[NamespaceIPFS-2]
	_ = x[NamespaceIPNS-3]
	_ = x[NamespaceFiles-4]
	_ = x[NamespacePinFS-5]
	_ = x[NamespaceKeyFS-6]
	_ = x[NamespaceAll-7]
	_ = x[NamespaceAllInOne-8]
}

const _Namespace_name = "NoneCoreIPFSIPNSFilesPinFSKeyFSAllOverlay"

var _Namespace_index = [...]uint8{0, 4, 8, 12, 16, 21, 26, 31, 34, 41}

func (i Namespace) String() string {
	if i < 0 || i >= Namespace(len(_Namespace_index)-1) {
		return "Namespace(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _Namespace_name[_Namespace_index[i]:_Namespace_index[i+1]]
}
