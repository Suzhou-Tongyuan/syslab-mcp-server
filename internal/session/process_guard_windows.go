//go:build windows

package session

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

const (
	processTerminate         = 0x0001
	processSetQuota          = 0x0100
	jobObjectExtendedInfo    = 9
	jobObjectLimitKillOnClose = 0x00002000
)

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var (
	jobKernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW        = jobKernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject = jobKernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = jobKernel32.NewProc("AssignProcessToJobObject")
	procCloseJobHandle          = jobKernel32.NewProc("CloseHandle")
	jobHandles                  sync.Map
)

func attachProcessLifetime(pid int) error {
	if pid <= 0 {
		return nil
	}
	jobHandle, err := createKillOnCloseJob()
	if err != nil {
		return err
	}
	processHandle, err := syscall.OpenProcess(processTerminate|processSetQuota, false, uint32(pid))
	if err != nil {
		closeHandle(jobHandle)
		return err
	}
	defer syscall.CloseHandle(processHandle)

	r1, _, e1 := procAssignProcessToJobObject.Call(uintptr(jobHandle), uintptr(processHandle))
	if r1 == 0 {
		closeHandle(jobHandle)
		if e1 != syscall.Errno(0) {
			return e1
		}
		return fmt.Errorf("AssignProcessToJobObject failed")
	}

	jobHandles.Store(pid, jobHandle)
	return nil
}

func releaseProcessLifetime(pid int) {
	if pid <= 0 {
		return
	}
	if value, ok := jobHandles.LoadAndDelete(pid); ok {
		if handle, ok := value.(syscall.Handle); ok {
			closeHandle(handle)
		}
	}
}

func createKillOnCloseJob() (syscall.Handle, error) {
	r1, _, e1 := procCreateJobObjectW.Call(0, 0)
	handle := syscall.Handle(r1)
	if handle == 0 {
		if e1 != syscall.Errno(0) {
			return 0, e1
		}
		return 0, fmt.Errorf("CreateJobObjectW failed")
	}

	info := jobObjectExtendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnClose
	r1, _, e1 = procSetInformationJobObject.Call(
		uintptr(handle),
		uintptr(jobObjectExtendedInfo),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)
	if r1 == 0 {
		closeHandle(handle)
		if e1 != syscall.Errno(0) {
			return 0, e1
		}
		return 0, fmt.Errorf("SetInformationJobObject failed")
	}
	return handle, nil
}

func closeHandle(handle syscall.Handle) {
	if handle != 0 {
		_, _, _ = procCloseJobHandle.Call(uintptr(handle))
	}
}
