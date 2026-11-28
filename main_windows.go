//go:build windows

package main

import (
	"context"
	"embed"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/energye/systray"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"pwdtt/backend"
	"pwdtt/internal/splittunnel"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/icons/icon.png
var appIcon []byte

//go:embed assets/icons/icon.ico
var trayIcon []byte

//go:embed assets/wintun.dll
var wintunDLL []byte

var (
	trayReady    atomic.Bool
	trayStopping atomic.Bool
	trayStop     = make(chan struct{})
	trayExited   = make(chan struct{})
	trayStopOnce sync.Once
	trayExitOnce sync.Once
)

func showMainWindow(ctx context.Context) {
	wailsruntime.WindowUnminimise(ctx)
	wailsruntime.WindowShow(ctx)
}

func startTray(ctx context.Context) {
	go systray.Run(func() {
		if trayStopping.Load() {
			systray.Quit()
			return
		}

		enableNativeMenuTheme()
		trayIcons := backend.BuildTrayIcons(appIcon, trayIcon)
		currentIconVariant := backend.TrayIconIdle
		systray.SetIcon(trayIcons[currentIconVariant])
		systray.SetTooltip("mwPWDTT_windows")
		// Левый клик / двойной клик — просто разворачиваем окно в текущем состоянии
		systray.SetOnClick(func(systray.IMenu) { showMainWindow(ctx) })
		systray.SetOnDClick(func(systray.IMenu) { showMainWindow(ctx) })

		mShow := systray.AddMenuItem("Открыть mwPWDTT_windows", "Развернуть окно приложения")
		mShow.Click(func() { showMainWindow(ctx) })

		systray.AddSeparator()

		mToggleConn := systray.AddMenuItem("Подключить", "Подключить или отключить туннель")
		mToggleConn.Click(func() {
			wailsruntime.EventsEmit(ctx, "tray:toggle_connect")
		})

		mProxyMode := systray.AddMenuItemCheckbox("Прокси mihomo (SOCKS5)", "Переключить режим SOCKS5-прокси для mihomo", false)
		mProxyMode.Click(func() {
			wailsruntime.EventsEmit(ctx, "tray:toggle_proxy")
		})

		mSplitMode := systray.AddMenuItemCheckbox("Split Tunneling", "Включить или выключить раздельное туннелирование", false)
		mSplitMode.Click(func() {
			wailsruntime.EventsEmit(ctx, "tray:toggle_split")
		})

		systray.AddSeparator()

		mForkSettings := systray.AddMenuItem("Настройки mwPWDTT", "Открыть настройки mwPWDTT_windows")
		mForkSettings.Click(func() {
			showMainWindow(ctx)
			wailsruntime.EventsEmit(ctx, "tray:navigate", "/mw-settings")
		})

		mOpenSplit := systray.AddMenuItem("Открыть Split Tunneling", "Перейти в раздел Split Tunneling")
		mOpenSplit.Click(func() {
			showMainWindow(ctx)
			wailsruntime.EventsEmit(ctx, "tray:navigate", "/split-tunnel")
		})

		systray.AddSeparator()

		mExit := systray.AddMenuItem("Выход", "Закрыть mwPWDTT_windows")
		mExit.Click(func() { wailsruntime.Quit(ctx) })

		backend.SetTrayStateHandler(func(st backend.TrayState) {
			if !trayReady.Load() {
				return
			}
			flushNativeMenuTheme()

			port := st.ProxyPort
			if port <= 0 {
				port = backend.DefaultProxyPort
			}

			switch st.TunnelState {
			case "connected":
				mToggleConn.SetTitle("Отключить")
				mToggleConn.Enable()
			case "connecting":
				mToggleConn.SetTitle("Отменить подключение...")
				mToggleConn.Enable()
			case "disconnecting":
				mToggleConn.SetTitle("Отключение...")
				mToggleConn.Disable()
			default:
				mToggleConn.SetTitle("Подключить")
				mToggleConn.Enable()
			}

			if st.ProxyMode {
				mProxyMode.Check()
				mProxyMode.SetTitle(fmt.Sprintf("Прокси mihomo (127.0.0.1:%d)", port))
			} else {
				mProxyMode.Uncheck()
				mProxyMode.SetTitle("Прокси mihomo (SOCKS5)")
			}

			if st.SplitSupported {
				mSplitMode.Show()
				if st.ProxyMode {
					mSplitMode.Uncheck()
					mSplitMode.Disable()
					mSplitMode.SetTitle("Split Tunneling (пауза — mihomo)")
				} else {
					mSplitMode.Enable()
					mSplitMode.SetTitle("Split Tunneling")
					if st.SplitEnabled {
						mSplitMode.Check()
					} else {
						mSplitMode.Uncheck()
					}
				}
			} else {
				mSplitMode.Hide()
			}

			tooltip := "mwPWDTT_windows — Отключён"
			if st.TunnelState == "connected" {
				if st.ProxyMode {
					tooltip = fmt.Sprintf("mwPWDTT_windows — Подключён (SOCKS5 :%d)", port)
				} else if st.SplitEnabled {
					tooltip = "mwPWDTT_windows — Подключён (Split Tunnel)"
				} else {
					tooltip = "mwPWDTT_windows — Подключён"
				}
			} else if st.TunnelState == "connecting" {
				tooltip = "mwPWDTT_windows — Подключение..."
			} else if st.ProxyMode {
				tooltip = fmt.Sprintf("mwPWDTT_windows — SOCKS5 DIRECT (:%d)", port)
			}
			systray.SetTooltip(tooltip)
			nextVariant := backend.SelectTrayIconVariant(st)
			if nextVariant != currentIconVariant {
				if icoBytes, ok := trayIcons[nextVariant]; ok && len(icoBytes) > 0 {
					systray.SetIcon(icoBytes)
					currentIconVariant = nextVariant
				}
			}
		})

		trayReady.Store(true)
		go hideMinimisedWindow(ctx)
	}, func() {
		trayReady.Store(false)
		backend.SetTrayStateHandler(nil)
		trayExitOnce.Do(func() { close(trayExited) })
		if !trayStopping.Load() {
			showMainWindow(ctx)
		}
	})
}

func hideMinimisedWindow(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-trayStop:
			return
		case <-ticker.C:
			if trayReady.Load() && wailsruntime.WindowIsMinimised(ctx) {
				wailsruntime.WindowHide(ctx)
			}
		}
	}
}

func stopTray() {
	trayStopping.Store(true)
	trayStopOnce.Do(func() { close(trayStop) })
	if trayReady.Load() {
		systray.Quit()
		select {
		case <-trayExited:
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func main() {
	enableNativeMenuTheme()
	backend.InitWintun(wintunDLL)
	app := backend.NewApp()
	splitTunnel := splittunnel.NewService()
	secondInstanceLaunch := make(chan struct{}, 1)

	err := wails.Run(&options.App{
		Title:       "mwPWDTT_windows",
		Width:       900,
		Height:      600,
		MinWidth:    800,
		MinHeight:   550,
		Frameless:   false,
		StartHidden: app.ShouldStartHidden(),
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup: func(ctx context.Context) {
			app.Startup(ctx)
			splitTunnel.Startup(ctx)
			startTray(ctx)
			go func() {
				for range secondInstanceLaunch {
					showMainWindow(ctx)
				}
			}()
		},
		OnShutdown: func(ctx context.Context) {
			stopTray()
			app.Shutdown(ctx)
			splitTunnel.Shutdown(ctx)
		},
		Bind: []interface{}{app, splitTunnel},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "pwdtt-windows-client",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				select {
				case secondInstanceLaunch <- struct{}{}:
				default:
				}
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.SystemDefault,
		},
	})
	if err != nil {
		panic(err)
	}
}
