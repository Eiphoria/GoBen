package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

const (
	CHANNELS   int = 2
	FRAME_RATE int = 48000
	FRAME_SIZE int = 960
	MAX_BYTES  int = (FRAME_SIZE * 2) * 2
)

func main() {
	fmt.Println("Launching GoBen...")
	envVarName := "DPP_TOKEN"
	token := os.Getenv(envVarName)
	if token == "" {
		fmt.Println("Cant retrieve env var: ", envVarName)
		os.Exit(1)
		// return
	}
	fmt.Println("Pass token check")
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("error creating Discord session:", err)
		return
	}
	fmt.Println("Create bot")
	defer s.Close()

	// Register messageCreate as a callback for the messageCreate events.
	fmt.Println("try add handler")
	s.AddHandler(messageCreate)

	fmt.Println("trying open websocket")
	err = s.Open()
	if err != nil {

		fmt.Println("error opening connection:", err)
		return
	}
	fmt.Println("after open websocket")
	fmt.Println("Started")
	<-make(chan struct{})

}

// This function will be called (due to AddHandler above) when the bot receives
// the "ready" event from Discord.

// This function will be called (due to AddHandler above) every time a new
// message is created on any channel that the autenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}
	fmt.Println("message: ", m.Content)
	// check if the message is "!airhorn"
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
				err = playSound(s, g.ID, vs.ChannelID)
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

func playSound(s *discordgo.Session, guildID, channelID string) (err error) {
	fmt.Println("Enter: func playSound")
	// Join the provided voice channel.
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		return err
	}
	ytSrcLink := "https://rr1---sn-4upjvh-qv3z.googlevideo.com/videoplayback?expire=1696900346&ei=mlAkZZXOMbe20u8PhMy-iAU&ip=5.153.158.77&id=o-AFv8N5Jf1m19gQ3z4mvXbrEwcS7ObsBHYsI7Ds3w0a0C&itag=251&source=youtube&requiressl=yes&mh=Ul&mm=31%2C29&mn=sn-4upjvh-qv3z%2Csn-3c27sn7e&ms=au%2Crdu&mv=m&mvi=1&pl=24&gcr=ua&initcwndbps=772500&spc=UWF9f811LzR8RcthhKoxK_8bx4BNdk6mqT7NRNuQDQ&vprv=1&svpuc=1&mime=audio%2Fwebm&gir=yes&clen=1159641&dur=65.741&lmt=1657130529487498&mt=1696878308&fvip=3&keepalive=yes&fexp=24007246&beids=24350018&c=ANDROID&txp=2318224&sparams=expire%2Cei%2Cip%2Cid%2Citag%2Csource%2Crequiressl%2Cgcr%2Cspc%2Cvprv%2Csvpuc%2Cmime%2Cgir%2Cclen%2Cdur%2Clmt&sig=AGM4YrMwRgIhANSkkwaOXF-JAZghWQPEBhvyb1TX4bPU3DJC3yntHNB0AiEA97bf0oi7Z2lOEdF9ZW2PnZx6D-aMUcIb0qBo_mkjJH4%3D&lsparams=mh%2Cmm%2Cmn%2Cms%2Cmv%2Cmvi%2Cpl%2Cinitcwndbps&lsig=AK1ks_kwRQIhAOAHsO7eA5-Qjpou8vrE2fW_l4IzXii36XZmVHl1EdscAiBSBVONuktP_KKrH0wQZ68bQe7-MKdK2y0MFCCum0IJDA%3D%3D"
	// link := make(chan []int16, 2)

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
	// Stop speaking when function is done or break
	defer vc.Speaking(false)
	defer vc.Disconnect()

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
