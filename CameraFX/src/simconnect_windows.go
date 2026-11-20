//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	recvIDException     = 1
	recvIDOpen          = 2
	recvIDQuit          = 3
	recvIDSimObjectData = 8

	dataTypeFloat64 = 4
	periodSimFrame  = 3
	objectIDUser    = 0
	unused          = 0xFFFFFFFF
)

// SimConnect is a minimal binding to the native SimConnect.dll.
type SimConnect struct {
	open, close, addDef, reqData, getNext *syscall.Proc
	h                                     uintptr
}

func LoadSimConnect(path string) (*SimConnect, error) {
	dll, err := syscall.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}
	s := &SimConnect{}
	for name, p := range map[string]**syscall.Proc{
		"SimConnect_Open": &s.open, "SimConnect_Close": &s.close,
		"SimConnect_AddToDataDefinition": &s.addDef, "SimConnect_RequestDataOnSimObject": &s.reqData,
		"SimConnect_GetNextDispatch": &s.getNext,
	} {
		if *p, err = dll.FindProc(name); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func cstr(s string) *byte { b, _ := syscall.BytePtrFromString(s); return b }

func (s *SimConnect) Connect(name string) error {
	n := cstr(name)
	hr, _, _ := s.open.Call(uintptr(unsafe.Pointer(&s.h)), uintptr(unsafe.Pointer(n)), 0, 0, 0, 0)
	runtime.KeepAlive(n)
	if int32(hr) < 0 {
		s.h = 0
		return fmt.Errorf("SimConnect_Open failed (0x%08X) - is MSFS running?", uint32(hr))
	}
	for _, v := range simVars {
		vn, vu := cstr(v.Name), cstr(v.Units)
		hr, _, _ = s.addDef.Call(s.h, 0, uintptr(unsafe.Pointer(vn)), uintptr(unsafe.Pointer(vu)),
			dataTypeFloat64, 0, unused)
		runtime.KeepAlive(vn)
		runtime.KeepAlive(vu)
		if int32(hr) < 0 {
			s.Close()
			return fmt.Errorf("AddToDataDefinition(%s) failed", v.Name)
		}
	}
	hr, _, _ = s.reqData.Call(s.h, 0, 0, objectIDUser, periodSimFrame, 0, 0, 0, 0)
	if int32(hr) < 0 {
		s.Close()
		return fmt.Errorf("RequestDataOnSimObject failed")
	}
	return nil
}

func (s *SimConnect) Close() {
	if s.h != 0 {
		s.close.Call(s.h)
		s.h = 0
	}
}

// Poll drains pending messages. onData gets each telemetry sample; returns quit=true if the sim closed.
func (s *SimConnect) Poll(onData func(Telemetry)) (quit bool) {
	for i := 0; i < 64; i++ {
		var p uintptr
		var size uint32
		hr, _, _ := s.getNext.Call(s.h, uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&size)))
		if int32(hr) < 0 || p == 0 {
			return false
		}
		buf := unsafe.Slice((*byte)(unsafe.Pointer(p)), size)
		id := *(*uint32)(unsafe.Pointer(&buf[8]))
		switch id {
		case recvIDQuit:
			return true
		case recvIDException:
			if size >= 16 {
				logf("SimConnect exception %d", *(*uint32)(unsafe.Pointer(&buf[12])))
			}
		case recvIDSimObjectData:
			need := 40 + 8*len(simVars)
			if int(size) >= need {
				var t Telemetry
				dst := unsafe.Slice((*float64)(unsafe.Pointer(&t)), len(simVars))
				for k := range dst {
					dst[k] = *(*float64)(unsafe.Pointer(&buf[40+8*k]))
				}
				onData(t)
			}
		}
	}
	return false
}
