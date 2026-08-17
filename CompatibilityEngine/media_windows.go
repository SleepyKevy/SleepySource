//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// SleepySource reads Windows' Global System Media Transport Controls (GSMTC)
// directly through WinRT. Keeping this detector in-process avoids spawning a
// shell or scripting engine and keeps media detection local to the machine.

var (
	combase                       = syscall.NewLazyDLL("combase.dll")
	procRoInitialize              = combase.NewProc("RoInitialize")
	procRoUninitialize            = combase.NewProc("RoUninitialize")
	procRoGetActivationFactory    = combase.NewProc("RoGetActivationFactory")
	procWindowsCreateString       = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString       = combase.NewProc("WindowsDeleteString")
	procWindowsGetStringRawBuffer = combase.NewProc("WindowsGetStringRawBuffer")
)

const (
	roInitMultithreaded = 1
	asyncStarted        = 0
	asyncCompleted      = 1
	asyncCanceled       = 2
	asyncError          = 3
)

type winGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	// Windows.Media.Control.IGlobalSystemMediaTransportControlsSessionManagerStatics
	iidGSMTCManagerStatics = winGUID{0x2050c4ee, 0x11a0, 0x57de, [8]byte{0xae, 0xd7, 0xc9, 0x7c, 0x70, 0x33, 0x82, 0x45}}
	// Windows.Foundation.IAsyncInfo
	iidAsyncInfo = winGUID{0x00000036, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

func mediaHRESULTFailed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func mediaHRESULTError(op string, hr uintptr) error {
	return fmt.Errorf("%s failed (HRESULT 0x%08X)", op, uint32(hr))
}

func comMethod(obj unsafe.Pointer, slot int) uintptr {
	if obj == nil {
		return 0
	}
	vtbl := *(*uintptr)(obj)
	if vtbl == 0 {
		return 0
	}
	return *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
}

func comCall(obj unsafe.Pointer, slot int, args ...uintptr) uintptr {
	fn := comMethod(obj, slot)
	if fn == 0 {
		return ^uintptr(0)
	}
	argv := make([]uintptr, 1, len(args)+1)
	argv[0] = uintptr(obj)
	argv = append(argv, args...)
	r1, _, _ := syscall.SyscallN(fn, argv...)
	return r1
}

func comRelease(obj unsafe.Pointer) {
	if obj != nil {
		_ = comCall(obj, 2)
	}
}

func comQueryInterface(obj unsafe.Pointer, iid *winGUID) (unsafe.Pointer, error) {
	if obj == nil {
		return nil, errors.New("nil COM object")
	}
	var out unsafe.Pointer
	hr := comCall(obj, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if mediaHRESULTFailed(hr) || out == nil {
		return nil, mediaHRESULTError("QueryInterface", hr)
	}
	return out, nil
}

func createHString(value string) (uintptr, error) {
	runes := syscall.StringToUTF16(value)
	length := len(runes)
	if length > 0 && runes[length-1] == 0 {
		length--
	}
	var h uintptr
	var p *uint16
	if len(runes) > 0 {
		p = &runes[0]
	}
	hr, _, _ := procWindowsCreateString.Call(
		uintptr(unsafe.Pointer(p)),
		uintptr(length),
		uintptr(unsafe.Pointer(&h)),
	)
	if mediaHRESULTFailed(hr) {
		return 0, mediaHRESULTError("WindowsCreateString", hr)
	}
	return h, nil
}

func deleteHString(h uintptr) {
	if h != 0 {
		procWindowsDeleteString.Call(h)
	}
}

func hStringToString(h uintptr) string {
	if h == 0 {
		return ""
	}
	var length uint32
	ptr, _, _ := procWindowsGetStringRawBuffer.Call(h, uintptr(unsafe.Pointer(&length)))
	if ptr == 0 || length == 0 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), int(length)))
}

func comGetHString(obj unsafe.Pointer, slot int) (string, error) {
	var h uintptr
	hr := comCall(obj, slot, uintptr(unsafe.Pointer(&h)))
	if mediaHRESULTFailed(hr) {
		return "", mediaHRESULTError("WinRT string getter", hr)
	}
	defer deleteHString(h)
	return hStringToString(h), nil
}

func comGetObject(obj unsafe.Pointer, slot int) (unsafe.Pointer, error) {
	var out unsafe.Pointer
	hr := comCall(obj, slot, uintptr(unsafe.Pointer(&out)))
	if mediaHRESULTFailed(hr) {
		return nil, mediaHRESULTError("WinRT object getter", hr)
	}
	return out, nil
}

func awaitAsyncObject(ctx context.Context, op unsafe.Pointer, timeout time.Duration) (unsafe.Pointer, error) {
	if op == nil {
		return nil, errors.New("nil WinRT async operation")
	}
	info, err := comQueryInterface(op, &iidAsyncInfo)
	if err != nil {
		return nil, err
	}
	defer comRelease(info)

	deadline := time.Now().Add(timeout)
	for {
		var status int32
		hr := comCall(info, 7, uintptr(unsafe.Pointer(&status)))
		if mediaHRESULTFailed(hr) {
			return nil, mediaHRESULTError("IAsyncInfo.Status", hr)
		}
		switch status {
		case asyncCompleted:
			var out unsafe.Pointer
			hr = comCall(op, 8, uintptr(unsafe.Pointer(&out)))
			if mediaHRESULTFailed(hr) || out == nil {
				return nil, mediaHRESULTError("IAsyncOperation.GetResults", hr)
			}
			return out, nil
		case asyncCanceled:
			return nil, errors.New("WinRT async operation was canceled")
		case asyncError:
			var code int32
			_ = comCall(info, 8, uintptr(unsafe.Pointer(&code)))
			return nil, fmt.Errorf("WinRT async operation failed (HRESULT 0x%08X)", uint32(code))
		case asyncStarted:
		default:
			return nil, fmt.Errorf("unexpected WinRT async status %d", status)
		}
		if time.Now().After(deadline) {
			return nil, errors.New("Windows media-session request timed out")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
	}
}

func requestGSMTCManager(ctx context.Context) (unsafe.Pointer, error) {
	className, err := createHString("Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager")
	if err != nil {
		return nil, err
	}
	defer deleteHString(className)

	var factory unsafe.Pointer
	hr, _, _ := procRoGetActivationFactory.Call(
		className,
		uintptr(unsafe.Pointer(&iidGSMTCManagerStatics)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if mediaHRESULTFailed(hr) || factory == nil {
		return nil, mediaHRESULTError("RoGetActivationFactory(GSMTC)", hr)
	}
	defer comRelease(factory)

	var op unsafe.Pointer
	hr = comCall(factory, 6, uintptr(unsafe.Pointer(&op))) // RequestAsync
	if mediaHRESULTFailed(hr) || op == nil {
		return nil, mediaHRESULTError("GSMTC RequestAsync", hr)
	}
	defer comRelease(op)
	return awaitAsyncObject(ctx, op, 5*time.Second)
}

func playbackStatus(session unsafe.Pointer) (int32, error) {
	info, err := comGetObject(session, 9) // GetPlaybackInfo
	if err != nil || info == nil {
		return 0, err
	}
	defer comRelease(info)
	var status int32
	hr := comCall(info, 7, uintptr(unsafe.Pointer(&status))) // PlaybackStatus
	if mediaHRESULTFailed(hr) {
		return 0, mediaHRESULTError("PlaybackStatus", hr)
	}
	return status, nil
}

func playbackStatusName(status int32) string {
	switch status {
	case 0:
		return "Closed"
	case 1:
		return "Opened"
	case 2:
		return "Changing"
	case 3:
		return "Stopped"
	case 4:
		return "Playing"
	case 5:
		return "Paused"
	default:
		return "Unknown"
	}
}

func selectMediaSession(manager unsafe.Pointer, s Settings) (unsafe.Pointer, string, int32, error) {
	view, err := comGetObject(manager, 7) // GetSessions
	if err != nil || view == nil {
		return nil, "", 0, err
	}
	defer comRelease(view)

	var size uint32
	hr := comCall(view, 7, uintptr(unsafe.Pointer(&size)))
	if mediaHRESULTFailed(hr) {
		return nil, "", 0, mediaHRESULTError("GSMTC session list size", hr)
	}

	var chosen unsafe.Pointer
	var chosenSource string
	var chosenStatus int32
	for i := uint32(0); i < size; i++ {
		var session unsafe.Pointer
		hr = comCall(view, 6, uintptr(i), uintptr(unsafe.Pointer(&session))) // GetAt
		if mediaHRESULTFailed(hr) || session == nil {
			continue
		}
		source, sourceErr := comGetHString(session, 6)
		if sourceErr != nil || !mediaSourceAllowed(source, s) {
			comRelease(session)
			continue
		}
		status, _ := playbackStatus(session)
		if chosen == nil || (chosenStatus != 4 && status == 4) {
			if chosen != nil {
				comRelease(chosen)
			}
			chosen, chosenSource, chosenStatus = session, source, status
			if status == 4 {
				break
			}
		} else {
			comRelease(session)
		}
	}
	return chosen, chosenSource, chosenStatus, nil
}

func readMediaTrack(ctx context.Context, manager unsafe.Pointer, s Settings) (Track, bool, error) {
	session, source, status, err := selectMediaSession(manager, s)
	if err != nil {
		return Track{}, false, err
	}
	if session == nil {
		return Track{}, false, nil
	}
	defer comRelease(session)

	var mediaOp unsafe.Pointer
	hr := comCall(session, 7, uintptr(unsafe.Pointer(&mediaOp))) // TryGetMediaPropertiesAsync
	if mediaHRESULTFailed(hr) || mediaOp == nil {
		return Track{}, false, mediaHRESULTError("TryGetMediaPropertiesAsync", hr)
	}
	mediaProps, err := awaitAsyncObject(ctx, mediaOp, 2*time.Second)
	comRelease(mediaOp)
	if err != nil {
		return Track{}, false, err
	}
	defer comRelease(mediaProps)

	title, _ := comGetHString(mediaProps, 6)
	artist, _ := comGetHString(mediaProps, 9)
	if strings.TrimSpace(artist) == "" {
		artist, _ = comGetHString(mediaProps, 8) // AlbumArtist fallback
	}

	var positionMS, durationMS int64
	timeline, timelineErr := comGetObject(session, 8) // GetTimelineProperties
	if timelineErr == nil && timeline != nil {
		var startTicks, endTicks, positionTicks int64
		if !mediaHRESULTFailed(comCall(timeline, 6, uintptr(unsafe.Pointer(&startTicks)))) &&
			!mediaHRESULTFailed(comCall(timeline, 7, uintptr(unsafe.Pointer(&endTicks)))) &&
			!mediaHRESULTFailed(comCall(timeline, 10, uintptr(unsafe.Pointer(&positionTicks)))) {
			if endTicks > startTicks {
				durationMS = (endTicks - startTicks) / 10_000
			}
			if positionTicks > startTicks {
				positionMS = (positionTicks - startTicks) / 10_000
			}
			if positionMS < 0 {
				positionMS = 0
			}
			if durationMS > 0 && positionMS > durationMS {
				positionMS = durationMS
			}
		}
		comRelease(timeline)
	}

	found := strings.TrimSpace(title) != "" || strings.TrimSpace(artist) != ""
	return Track{
		Found:       found,
		Artist:      strings.TrimSpace(artist),
		Title:       strings.TrimSpace(title),
		Status:      playbackStatusName(status),
		Source:      strings.TrimSpace(source),
		PositionMS:  positionMS,
		DurationMS:  durationMS,
		SampledAtMS: time.Now().UnixMilli(),
	}, found, nil
}

func runPlatformMediaDetector(ctx context.Context, app *App) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procRoInitialize.Call(roInitMultithreaded)
	if mediaHRESULTFailed(hr) {
		app.updateTrack(Track{}, mediaHRESULTError("RoInitialize", hr).Error())
		<-ctx.Done()
		return
	}
	defer procRoUninitialize.Call()

	manager, err := requestGSMTCManager(ctx)
	if err != nil {
		app.updateTrack(Track{}, "Native Windows media-session detection unavailable: "+err.Error())
		<-ctx.Done()
		return
	}
	defer comRelease(manager)

	poll := func() {
		app.mu.RLock()
		s := app.settings
		app.mu.RUnlock()
		track, found, err := readMediaTrack(ctx, manager, s)
		if err != nil {
			if ctx.Err() == nil {
				app.updateTrack(Track{}, "Native Windows media-session detector error: "+err.Error())
			}
			return
		}
		if !found {
			app.updateTrack(Track{}, "Native Windows media-session detector ready — waiting for an eligible media session.")
			return
		}
		app.updateTrack(track, "Native Windows media-session detector connected.")
	}

	poll()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}
