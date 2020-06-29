package main

import (
	"flag"
	"log"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/libretro/ludo/audio"
	"github.com/libretro/ludo/core"
	_ "github.com/libretro/ludo/ggpo/ggpolog"
	"github.com/libretro/ludo/ggpo/ggponet"
	"github.com/libretro/ludo/history"
	"github.com/libretro/ludo/input"
	"github.com/libretro/ludo/menu"
	"github.com/libretro/ludo/netplay"
	ntf "github.com/libretro/ludo/notifications"
	"github.com/libretro/ludo/ping"
	"github.com/libretro/ludo/playlists"
	"github.com/libretro/ludo/scanner"
	"github.com/libretro/ludo/settings"
	"github.com/libretro/ludo/state"
	"github.com/libretro/ludo/video"
	"github.com/sirupsen/logrus"
)

func init() {
	// GLFW event handling must run on the main OS thread
	runtime.LockOSThread()
}

func runLoop(vid *video.Video, m *menu.Menu) {
	var currTime, prevTime time.Time
	for !vid.Window.ShouldClose() {
		currTime = time.Now()
		dt := float32(currTime.Sub(prevTime)) / 1000000000
		glfw.PollEvents()
		m.ProcessHotkeys()
		ntf.Process(dt)
		vid.ResizeViewport()

		netplay.Idle()
		netplay.RunFrame()
		if !state.Global.MenuActive && netplay.Synchronized {
			if state.Global.CoreRunning {
				state.Global.Core.Run()

				if state.Global.Core.FrameTimeCallback != nil {
					state.Global.Core.FrameTimeCallback.Callback(state.Global.Core.FrameTimeCallback.Reference)
				}
				if state.Global.Core.AudioCallback != nil {
					state.Global.Core.AudioCallback.Callback()
				}
			}
			vid.Render()
		} else {
			input.Poll()
			m.Update(dt)
			vid.Render()
			m.Render(dt)
		}
		m.RenderNotifications()
		if state.Global.FastForward {
			glfw.SwapInterval(0)
		} else {
			glfw.SwapInterval(1)
		}
		vid.Window.SwapBuffers()
		prevTime = currTime
	}
}

func main() {
	err := settings.Load()
	if err != nil {
		log.Println("[Settings]: Loading failed:", err)
		log.Println("[Settings]: Using default settings")
	}

	flag.StringVar(&state.Global.CorePath, "L", "", "Path to the libretro core")
	flag.BoolVar(&state.Global.Verbose, "v", false, "Verbose logs")
	flag.BoolVar(&state.Global.LudOS, "ludos", false, "Expose the features related to LudOS")
	numPlayers := flag.Int("n", 0, "Number of players")
	flag.Parse()
	args := flag.Args()
	playersIP := make([]string, *numPlayers)
	localPort := ""

	var gamePath string
	if len(args) > 0 {
		gamePath = args[0]
		if *numPlayers > 1 {
			for i := 1; i < len(args)-1; i++ {
				playersIP[i-1] = args[i]
			}
			localPort = args[len(args)-1]
		}
	}

	if err := glfw.Init(); err != nil {
		log.Fatalln("Failed to initialize glfw", err)
	}
	defer glfw.Terminate()

	state.Global.DB, err = scanner.LoadDB(settings.Current.DatabaseDirectory)
	if err != nil {
		log.Println("Can't load game database:", err)
	}

	playlists.Load()

	history.Load()

	vid := video.Init(settings.Current.VideoFullscreen)

	audio.Init()

	m := menu.Init(vid)

	core.Init(vid)

	if *numPlayers > 1 {
		delay := int64(math.Round(float64(ping.GetAvgPing("8.8.8.8")) / float64(1000.0/60.0)))
		if delay >= netplay.MAX_FRAME_DELAY {
			netplay.FRAME_DELAY = netplay.MAX_FRAME_DELAY
		} else if delay > 0 {
			netplay.FRAME_DELAY = delay
		}
		InitNetwork(*numPlayers, playersIP, localPort)
	}

	input.Init(vid)

	if len(state.Global.CorePath) > 0 {
		err := core.Load(state.Global.CorePath)
		if err != nil {
			panic(err)
		}
	}

	if len(gamePath) > 0 {
		err := core.LoadGame(gamePath)
		if err != nil {
			ntf.DisplayAndLog(ntf.Error, "Menu", err.Error())
		} else {
			m.WarpToQuickMenu()
		}
	}

	// No game running? display the menu
	state.Global.MenuActive = !state.Global.CoreRunning

	runLoop(vid, m)

	// Unload and deinit in the core.
	core.Unload()
}

// ./ludo -n=2 -L cores/snes9x_libretro.dll B:/Downloads/Street_Fighter_II_Turbo_U.smc local 127.0.0.1:8090 8089
// ./ludo -n=2 -L cores/snes9x_libretro.dll B:/Downloads/Street_Fighter_II_Turbo_U.smc 127.0.0.1:8089 local 8090

func InitNetwork(numPlayers int, playersIP []string, localPort string) {
	players := make([]ggponet.GGPOPlayer, ggponet.GGPO_MAX_SPECTATORS+ggponet.GGPO_MAX_PLAYERS)

	for i := 0; i < numPlayers; i++ {
		players[i].PlayerNum = int64(i + 1)
		if playersIP[i] == "local" {
			players[i].Type = ggponet.GGPO_PLAYERTYPE_LOCAL
			continue
		}
		players[i].Type = ggponet.GGPO_PLAYERTYPE_REMOTE
		players[i].IPAddress = strings.Split(playersIP[i], ":")[0]
		port, err := strconv.Atoi(strings.Split(playersIP[i], ":")[1])
		players[i].Port = uint64(port)
		if err != nil {
			logrus.Panic("Error in InitNetwork")
		}
	}
	//TODO: Spectators

	netplay.Init(int64(numPlayers), players, localPort, 0, false)
}
