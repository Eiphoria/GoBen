package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	"github.com/kkdai/youtube/v2"
)

const (
	CHANNELS   int = 2
	FRAME_RATE int = 48000
	FRAME_SIZE int = 960
	MAX_BYTES  int = (FRAME_SIZE * 2) * 2
)

func logMessage(message string, level string) {
	_, fn, line, _ := runtime.Caller(1)
	log.Printf("%s | %s:%d | [%s] | %s\n", getTimeStamp(), fn, line, level, message)
	fmt.Printf("%s | %s:%d | [%s] | %s\n", getTimeStamp(), fn, line, level, message)
}

func getTimeStamp() string {
	t := time.Now()
	return t.Format("01/02/2006 15:04:05.000")
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
		return "", fmt.Errorf("queue is empty")
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

	f, err := os.OpenFile("file.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()
	log.SetFlags(0)
	log.SetOutput(f)

	logMessage("Launching GoBen...", "INFO")
	envVarName := "DPP_TOKEN"
	token := os.Getenv(envVarName)
	if token == "" {
		fmt.Println("Cant retrieve env var: ", envVarName)
		os.Exit(1)
	}

	logMessage("Pass token check", "INFO")
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("error creating Discord session:", err)
		return
	}

	logMessage("Bot created", "INFO")
	defer s.Close()

	logMessage("Add handlers", "INFO")
	s.AddHandler(messageCreate)

	logMessage("Open WebSocket", "INFO")
	err = s.Open()
	if err != nil {
		logMessage("error opening connection: "+err.Error(), "ERROR")
		return
	}

	logMessage("Bot Started", "INFO")
	<-make(chan struct{})

}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	if m.Author.ID == s.State.User.ID {
		return
	}

	if strings.HasPrefix(m.Content, ".play") {
		url := extractYouTubeURL(m.Content)
		if url == "" {
			logMessage("Could not extract url ", "ERROR")

			return
		}
		if _, ok := queueMap[m.GuildID]; !ok {
			queueMap[m.GuildID] = Queue{}
		}
		queue := queueMap[m.GuildID]
		queue.Enqueue(url)
		queueMap[m.GuildID] = queue

		voiceconn := s.VoiceConnections[m.GuildID]
		if voiceconn != nil {
			return
		}
		c, err := s.State.Channel(m.ChannelID)
		if err != nil {
			logMessage("Could not find channel: "+err.Error(), "ERROR")
			return
		}

		g, err := s.State.Guild(c.GuildID)
		if err != nil {
			logMessage("Could not find guild: "+err.Error(), "ERROR")
			return
		}
		for _, vs := range g.VoiceStates {
			if vs.UserID == m.Author.ID {
				if err = playSound(s, g.ID, vs.ChannelID); err != nil {
					logMessage("Error playing sound: "+err.Error(), "ERROR")
				}
				return
			}
		}
		logMessage(m.Author.ID+" not found in guild.VoiceStates.", "ERROR")

	}
	if strings.HasPrefix(m.Content, ".stop") {
		voiceconn := s.VoiceConnections[m.GuildID]
		if voiceconn == nil {
			return
		}
		delete(queueMap, m.GuildID)
		voiceconn.Disconnect()
	}

	if strings.HasPrefix(m.Content, ".queue") {
		queue, ok := queueMap[m.GuildID]
		if !ok {
			fmt.Println("No queue found for this guild.")
			return
		}

		items, _ := queue.ShowAll()
		str := strings.Join(items, "|\n")
		logMessage(".queue cmd call,queue have entry: "+str, "DEBUG")
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

func extractYouTubeURL(input string) string {

	input = strings.TrimPrefix(input, ".play ")

	re := regexp.MustCompile(`https://www\.youtube\.com/watch\?v=[a-zA-Z0-9_-]+`)
	match := re.FindString(input)

	if match != "" {
		return match
	}

	return ""
}

func playSound(s *discordgo.Session, guildID, channelID string) error {
	for {
		queue := queueMap[guildID]

		vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
		if err != nil {
			logMessage("error join voicechannel: "+err.Error(), "ERROR")
			return fmt.Errorf("error join voicechannel: %w", err)
		}

		url, err := queue.Dequeue()
		if err != nil {
			if err.Error() == "queue is empty" {
				logMessage("Queue is empty. Ending playback.", "INFO")
				defer vc.Speaking(false)
				defer vc.Disconnect()
				delete(queueMap, guildID)
				return nil
			}
		}
		queueMap[guildID] = queue

		options := dca.StdEncodeOptions
		options.FrameDuration = 20
		options.Bitrate = 128
		options.Application = "lowdelay"

		ctx := context.Background()
		client := youtube.Client{}
		ytvideo, err := client.GetVideoContext(ctx, url)
		if err != nil {
			logMessage("error getvideocontext(): "+err.Error(), "ERROR")
			return fmt.Errorf("error getvideocontext(): %w", err)

		}
		video, err := client.GetVideo(ytvideo.ID)
		if err != nil {
			logMessage("error getvideo(): "+err.Error(), "ERROR")
			return fmt.Errorf("error getvideo(): %w", err)
		}
		formats := video.Formats.WithAudioChannels().Itag(251)
		stream, _, err := client.GetStream(video, &formats[0])
		if err != nil {
			logMessage("error getstream(): "+err.Error(), "ERROR")
			return fmt.Errorf("error getstream(): %w", err)
		}
		encodingSession, err := dca.EncodeMem(stream, options)
		if err != nil {
			logMessage("error in encodefile(): "+err.Error(), "ERROR")
			return fmt.Errorf("error in encodefile(): %s", err)

		}
		defer encodingSession.Cleanup()
		done := make(chan error)
		dca.NewStream(encodingSession, vc, done)
		err = <-done
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			logMessage("i dont know what is it error about: "+err.Error(), "ERROR")
			return fmt.Errorf("i dont know what is it error about: %s", err)
		}
	}
}
