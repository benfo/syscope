package metrics

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A minimal binding to the Performance Data Helper API, which serves the
// counters Task Manager and perfmon show (e.g. per-engine GPU usage).

var (
	pdh                          = windows.NewLazySystemDLL("pdh.dll")
	procPdhOpenQuery             = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounter     = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData      = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterAr = pdh.NewProc("PdhGetFormattedCounterArrayW")
)

const (
	pdhFmtDouble   = 0x00000200
	pdhFmtNoCap100 = 0x00008000
	pdhMoreData    = 0x800007D2
)

// pdhItem mirrors PDH_FMT_COUNTERVALUE_ITEM_W with a double value.
type pdhItem struct {
	name   *uint16
	status uint32
	_      uint32
	value  float64
}

type pdhQuery struct {
	h        uintptr
	counters map[string]uintptr
}

// newPDHQuery opens a query with English counter paths (locale-independent).
func newPDHQuery(paths ...string) (*pdhQuery, error) {
	q := &pdhQuery{counters: map[string]uintptr{}}
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&q.h))); r != 0 {
		return nil, errors.New("PdhOpenQuery failed")
	}
	for _, p := range paths {
		p16, err := windows.UTF16PtrFromString(p)
		if err != nil {
			return nil, err
		}
		var c uintptr
		if r, _, _ := procPdhAddEnglishCounter.Call(q.h, uintptr(unsafe.Pointer(p16)), 0, uintptr(unsafe.Pointer(&c))); r == 0 {
			q.counters[p] = c
		}
	}
	if len(q.counters) == 0 {
		return nil, errors.New("no PDH counters available")
	}
	return q, nil
}

// collect takes a sample. Rate counters need two collections before they
// have values.
func (q *pdhQuery) collect() error {
	if r, _, _ := procPdhCollectQueryData.Call(q.h); r != 0 {
		return errors.New("PdhCollectQueryData failed")
	}
	return nil
}

// values returns instance name → value for a wildcard counter path.
func (q *pdhQuery) values(path string) map[string]float64 {
	c, ok := q.counters[path]
	if !ok {
		return nil
	}
	var size, count uint32
	r, _, _ := procPdhGetFormattedCounterAr.Call(c, pdhFmtDouble|pdhFmtNoCap100,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if r != pdhMoreData || size == 0 {
		return nil
	}
	// One allocation holds the items and the instance-name strings they
	// point to; size it in whole items.
	items := make([]pdhItem, int(size)/int(unsafe.Sizeof(pdhItem{}))+1)
	r, _, _ = procPdhGetFormattedCounterAr.Call(c, pdhFmtDouble|pdhFmtNoCap100,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&items[0])))
	if r != 0 {
		return nil
	}
	out := make(map[string]float64, count)
	for _, it := range items[:count] {
		if it.status <= 1 { // PDH_CSTATUS_VALID_DATA / NEW_DATA
			out[windows.UTF16PtrToString(it.name)] += it.value
		}
	}
	return out
}
