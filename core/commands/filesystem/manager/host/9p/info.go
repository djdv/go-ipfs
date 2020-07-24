package p9fsp

import ninelib "github.com/hugelgupf/p9/p9"

func (f *fid) StatFS() (stat ninelib.FSStat, err error) {
	stat.FSID = uint64(f.nodeInterface.ID())
	return
}

func (f *fid) WalkGetAttr(components []string) (qids []ninelib.QID, file ninelib.File, filled ninelib.AttrMask, attr ninelib.Attr, err error) {
	if qids, file, err = f.Walk(components); err != nil {
		return
	}

	_, filled, attr, err = file.GetAttr(ninelib.AttrMaskAll)
	return
}

func (f *fid) GetAttr(req ninelib.AttrMask) (ninelib.QID, ninelib.AttrMask, ninelib.Attr, error) {
	f.log.Debugf("GetAttr: %s", f.path.String())

	fidInfo, infoFilled, err := f.nodeInterface.Info(f.path.String(), requestFrom9P(req))
	if err != nil {
		return f.QID, ninelib.AttrMask{}, ninelib.Attr{}, interpretError(err)
	}

	attr := attrFromCore(fidInfo) // TODO: maybe resolve IDs
	tg := timeGroup{atime: f.initTime, mtime: f.initTime, ctime: f.initTime, btime: f.initTime}
	applyCommonsToAttr(&attr, f.filesWritable, tg, idGroup{uid: ninelib.NoUID, gid: ninelib.NoGID})

	return f.QID, filledFromCore(infoFilled), attr, nil
}
