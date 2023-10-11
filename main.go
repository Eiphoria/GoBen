package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"golang.org/x/exp/slog"
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

var queueMap = make(map[string]Queue)

type Queue struct {
	items []string
}

func (q *Queue) Enqueue(item string) {
	q.items = append(q.items, item)
}

func (q *Queue) Dequeue() (string, error) {
	if len(q.items) == 0 {
		return "", fmt.Errorf("Queue is empty")
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, nil
}

func (q *Queue) IsEmpty() bool {
	return len(q.items) == 0
}
func (q *Queue) ShowAll() ([]string, error) {
	if len(q.items) == 0 {
		return []string{""}, fmt.Errorf("Queue is empty")
	}
	return q.items, nil
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
		queue := queueMap[m.GuildID]
		// if queue.IsEmpty() {
		// 	return
		// }

		url := extractYouTubeURL(m.Content)
		if url == "" {
			return
		}
		queue.Enqueue(url)
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

				if err = playSound(s, g.ID, vs.ChannelID, m.Content); err != nil {
					log.Println("error playing sound:", err.Error())
				}

				return
			}
		}
		fmt.Println(m.Author.ID, " not found in guild.VoiceStates")

	}
	if strings.HasPrefix(m.Content, ".stop") {
		// queue := queueMap[m.GuildID]
		// if queue.IsEmpty() {
		// 	return
		// }
		delete(queueMap, m.GuildID)
		voiceconn := s.VoiceConnections[m.GuildID]
		voiceconn.Disconnect()
	}

	if strings.HasPrefix(m.Content, ".queue") {
		queue := queueMap[m.GuildID]
		if queue.IsEmpty() {
			return
		}
		list := "List of songs:\n"
		songs, err := queue.ShowAll()
		if err != nil {
			return
		}

		for i, song := range songs {
			list += strconv.Itoa(i) + " " + song + "\n"
		}

		s.ChannelMessageSend(m.ChannelID, list)
	}
}

// func sendPCM(voice *discordgo.VoiceConnection, pcm <-chan []int16) {

// 	encoder, err := gopus.NewEncoder(FRAME_RATE, CHANNELS, gopus.Audio)
// 	if err != nil {
// 		fmt.Println("NewEncoder error,", err)
// 		return
// 	}

// 	for {
// 		receive, ok := <-pcm
// 		if !ok {
// 			fmt.Println("PCM channel closed")
// 			return
// 		}
// 		opus, err := encoder.Encode(receive, FRAME_SIZE, MAX_BYTES)
// 		if err != nil {
// 			fmt.Println("Encoding error,", err)
// 			return
// 		}
// 		// fmt.Println(!voice.Ready, voice.OpusSend == nil)
// 		if !voice.Ready || voice.OpusSend == nil {
// 			fmt.Printf("Discordgo not ready for opus packets. %+v : %+v", voice.Ready, voice.OpusSend)
// 			return
// 		}
// 		voice.OpusSend <- opus
// 	}
// }

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

func playSound(s *discordgo.Session, guildID, channelID string, message string) error {
	fmt.Println("Enter: func playSound")
	// queue := queueMap[guildID]
	// Join the provided voice channel.
	url := extractYouTubeURL(message)
	if url == "" {
		return nil
	}

	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}

	defer vc.Disconnect()

	ytdl := exec.Command("youtube-dl", "-v", "-f", "bestaudio", "-o", "-", url)

	ytdlout, err := ytdl.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ytdl stdout pipe: %w", err)
	}
	ytdlbuf := bufio.NewReaderSize(ytdlout, 16384)

	ffmpeg := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpeg.Stdin = ytdlbuf
	out, err := ffmpeg.StdoutPipe()
	if err != nil {
		return err
	}

	dca := exec.Command("dca.exe", "pipe:0")
	dca.Stdin = bufio.NewReaderSize(out, 1024)
	dcaout, err := dca.StdoutPipe()
	if err != nil {
		return fmt.Errorf("dca stdout pipe: %w", err)
	}

	if err = ytdl.Start(); err != nil {
		return fmt.Errorf("ytdl start: %w", err)
	}

	defer ytdl.Wait()

	if err = ffmpeg.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	defer ffmpeg.Wait()

	if err = dca.Start(); err != nil {
		return fmt.Errorf("dca start: %w", err)
	}

	defer dca.Wait()

	// header "buffer"
	var opuslen int16

	// Send "speaking" packet over the voice websocket
	vc.Speaking(true)

	// Send not "speaking" packet over the websocket when we finish
	defer vc.Speaking(false)

	dcaBuf := bufio.NewReaderSize(dcaout, 1024)
	for {
		if err = binary.Read(dcaBuf, binary.LittleEndian, &opuslen); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}

			return fmt.Errorf("binary read: %w", err)
		}

		// read opus data from dca
		opus := make([]byte, opuslen)
		if err = binary.Read(dcaBuf, binary.LittleEndian, &opus); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}

			if err != nil {
				return fmt.Errorf("binary read: %w", err)
			}
		}

		// Send received PCM to the sendPCM channel
		vc.OpusSend <- opus
	}
}
