package main

import (
	"fmt"
	"os/exec"
)

func main() {
	sourceURL, err := exec.Command("youtube-dl", "--get-url", "--format", "bestaudio", "https://www.youtube.com/watch?v=RGvKLa0CYzE").Output()
	if err != nil {
		panic(err)
	}

	fmt.Println(string(sourceURL))
}
