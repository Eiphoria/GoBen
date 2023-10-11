package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"golang.org/x/exp/slog"
	"layeh.com/gopus"
)

const (
	CHANNELS   int = 2
	FRAME_RATE int = 48000
	FRAME_SIZE int = 960
	MAX_BYTES  int = (FRAME_SIZE * 2) * 2
)

func Infof(logger *slog.Logger, format string, args ...any) {
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf(format, args...), pcs[0])
	_ = logger.Handler().Handle(context.Background(), r)
}

func main() {
	replace := func(groups []string, a slog.Attr) slog.Attr {
		// Remove time.
		if a.Key == slog.TimeKey && len(groups) == 0 {
			return slog.Attr{}
		}
		// Remove the directory from the source's filename.
		if a.Key == slog.SourceKey {
			source := a.Value.Any().(*slog.Source)
			source.File = filepath.Base(source.File)
		}
		return a
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource:   true,
		Level:       nil,
		ReplaceAttr: replace,
	}))
	Infof(logger, "[INFO]: %s", "Launching GoBen...")
	envVarName := "DPP_TOKEN"
	token := os.Getenv(envVarName)
	if token == "" {
		fmt.Println("Cant retrieve env var: ", envVarName)
		os.Exit(1)
		// return
	}
	Infof(logger, "[INFO]: %s", "Pass token check")
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("error creating Discord session:", err)
		return
	}
	Infof(logger, "[INFO]: %s", "Bot created")
	defer s.Close()

	// Register messageCreate as a callback for the messageCreate events.
	Infof(logger, "[INFO]: %s", "Add handlers")
	s.AddHandler(messageCreate)

	Infof(logger, "[INFO]: %s", "Open WebSocket")
	err = s.Open()
	if err != nil {

		fmt.Println("error opening connection:", err)
		return
	}

	Infof(logger, "[INFO]: %s", "Bot Started")
	<-make(chan struct{})

}

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// It's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// check if the message contain ".play" command
	if strings.HasPrefix(m.Content, ".play") {

		fmt.Println("message: ", m.Content)
		// Find the channel that the message came from.
		c, err := s.State.Channel(m.ChannelID)
		if err != nil {
			// Could not find channel.
			fmt.Println("message: Could not find channel")
			return
		}
		fmt.Println("Channel found")
		// Find the guild for that channel.
		g, err := s.State.Guild(c.GuildID)
		if err != nil {
			// Could not find guild.
			fmt.Println("message: Could not find guild")
			return
		}
		fmt.Println("Guild found")
		// Look for the message sender in that guild's current voice states.
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				fmt.Println(vs.UserID, " == ", m.Author.ID)
				err = playSound(s, g.ID, vs.ChannelID, m.Content)
				if err != nil {
					fmt.Println("Error playing sound:", err)
				}

				return
			}
		}
		fmt.Println(m.Author.ID, " not found in guild.VoiceStates")
		// vc.Disconnect()
	}
}

func sendPCM(voice *discordgo.VoiceConnection, pcm <-chan []int16) {

	encoder, err := gopus.NewEncoder(FRAME_RATE, CHANNELS, gopus.Audio)
	if err != nil {
		fmt.Println("NewEncoder error,", err)
		return
	}
	for {
		receive, ok := <-pcm
		if !ok {
			fmt.Println("PCM channel closed")
			return
		}
		opus, err := encoder.Encode(receive, FRAME_SIZE, MAX_BYTES)
		if err != nil {
			fmt.Println("Encoding error,", err)
			return
		}
		if !voice.Ready || voice.OpusSend == nil {
			fmt.Printf("Discordgo not ready for opus packets. %+v : %+v", voice.Ready, voice.OpusSend)
			return
		}
		voice.OpusSend <- opus
	}
}

func extractYouTubeURL(input string) string {
	// Убираем .play из строки
	input = strings.TrimPrefix(input, ".play ")

	// Используем регулярное выражение для поиска ссылки на YouTube
	re := regexp.MustCompile(`https://www\.youtube\.com/watch\?v=[a-zA-Z0-9_-]+`)
	match := re.FindString(input)

	if match != "" {
		return match
	}

	return ""
}

func getYoutubeAudioURL(input string) string {
	cmd := exec.Command("youtube-dl", "--get-url", "--format", "bestaudio", input)

	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Ошибка выполнения команды:", err)
		return ""
	}

	url := strings.TrimSpace(string(output))
	if strings.HasPrefix(url, "https://") {
		return url
	}

	return ""
}

func playSound(s *discordgo.Session, guildID, channelID string, message string) (err error) {
	fmt.Println("Enter: func playSound")

	// Join the provided voice channel.

	url := extractYouTubeURL(message)
	if url == "" {
		return
	}

	ytSrcLink := getYoutubeAudioURL(url)
	if ytSrcLink == "" {
		return
	}

	// ytSrcLink := "https://rr1---sn-4upjvh-qv3z.googlevideo.com/videoplayback?expire=1697053539&ei=A6cmZZKAGc3ryAWpnKeABw&ip=5.153.158.77&id=o-AA1CJy595sZvwYkV1Wn0M_7kKhLJxY1mAAL8UXppMBET&itag=251&source=youtube&requiressl=yes&mh=Ul&mm=31%2C29&mn=sn-4upjvh-qv3z%2Csn-3c27sn7e&ms=au%2Crdu&mv=m&mvi=1&pl=24&gcr=ua&initcwndbps=908750&spc=UWF9fwBaJOKgJ6NmtXMw4Bny1Y0JCS69FKy-K1t6Xw&vprv=1&svpuc=1&mime=audio%2Fwebm&gir=yes&clen=1159641&dur=65.741&lmt=1657130529487498&mt=1697031494&fvip=3&keepalive=yes&fexp=24007246&beids=24472435&c=ANDROID&txp=2318224&sparams=expire%2Cei%2Cip%2Cid%2Citag%2Csource%2Crequiressl%2Cgcr%2Cspc%2Cvprv%2Csvpuc%2Cmime%2Cgir%2Cclen%2Cdur%2Clmt&sig=AGM4YrMwRAIgBVOrCPOZAGDtg5MwG-kG5amDdvwlxC5tL8OcQPV60UMCIDtZ6LXMPnC6cTp8C3uCrzp6ClHC8HKCK5PIbP8ZKAcA&lsparams=mh%2Cmm%2Cmn%2Cms%2Cmv%2Cmvi%2Cpl%2Cinitcwndbps&lsig=AK1ks_kwRAIgI9jLyocL8gzMMACA606CWuVyDmCpcxjCrb2_V-kQFPICICKk7AHVoV3h6u4LIhPzcxWuU8qx7HvSa9N4Zol-iiNm"
	// link := make(chan []int16, 2)

	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}

	defer vc.Disconnect()

	ffmpeg := exec.Command("ffmpeg", "-i", ytSrcLink, "-f", "s16le", "-ar", "48000", "-ac",
		"2", "pipe:1")

	out, err := ffmpeg.StdoutPipe()
	if err != nil {
		return err
	}
	err = ffmpeg.Start()
	buffer := bufio.NewReaderSize(out, 16384)

	if err != nil {
		return err
	}

	send := make(chan []int16, 2)

	// Start speaking.
	vc.Speaking(true)
	go sendPCM(vc, send)
	defer vc.Speaking(false)
	// Stop speaking when function is done or break

	for {

		audioBuffer := make([]int16, FRAME_SIZE*CHANNELS)
		err = binary.Read(buffer, binary.LittleEndian, &audioBuffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
		send <- audioBuffer
	}

	// Disconnect from the provided voice channel.

	// return nil
}
