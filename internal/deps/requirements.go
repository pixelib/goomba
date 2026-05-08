package deps

type Requirements struct {
	NeedZig    bool
	NeedMacSDK bool
	CgoEnabled bool
}

func (r Requirements) Any() bool {
	return r.NeedZig || r.NeedMacSDK
}

func (r Requirements) Count() int {
	count := 0
	if r.NeedZig {
		count++
	}
	if r.NeedMacSDK {
		count++
	}
	return count
}
