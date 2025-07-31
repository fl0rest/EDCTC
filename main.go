package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	serverURL    = "http://localhost:8000/api/save"
	pollInterval = 5 * time.Second
)

var lastModTime time.Time

func main() {
	journalDir := getJournalDir()
	var lastFile string
	var lastOffset int64

	for {
		latestFile, _, err := findLatestFile(journalDir)
		if err != nil {
			fmt.Println("Error finding latest file:", err)
			time.Sleep(pollInterval)
			continue
		}

		if latestFile != lastFile {
			lastFile = latestFile
			lastOffset = 0
			fmt.Println("Now watching:", latestFile)
		}

		lastOffset, err = readNewLines(latestFile, lastOffset, sendEventToServer)
		if err != nil {
			fmt.Println(err)
		}
		time.Sleep(pollInterval)
	}
}

func sendEventToServer(line string) {
	resp, err := http.Post(serverURL, "application/json", bytes.NewBuffer([]byte(line)))
	fmt.Println(line)
	if err != nil {
		fmt.Println("Send error:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Not 200: %s", resp.Status)
	}
}

func getJournalDir() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "Saved Games", "Frontier Developments", "Elite Dangerous")
	}

	return "Journal.log"
}

func readNewLines(path string, lastSize int64, onLine func(string)) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return lastSize, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return lastSize, err
	}

	if stat.Size() == lastSize {
		// No change
		return lastSize, nil
	}

	// Seek to last read position
	_, err = file.Seek(lastSize, io.SeekStart)
	if err != nil {
		return lastSize, err
	}

	scanner := bufio.NewScanner(file)
	var lastMatch string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "ColonisationConstructionDepot") &&
			strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			lastMatch = line
		}
	}

	if err := scanner.Err(); err != nil {
		return lastSize, err
	}

	// Only send the last matching line
	if lastMatch != "" {
		onLine(lastMatch)
	}

	return stat.Size(), nil
}

func findLatestFile(path string) (string, time.Time, error) {
	var latestFile string
	var latestMod time.Time

	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if !strings.HasPrefix(info.Name(), "Journal") || !strings.HasSuffix(info.Name(), ".log") {
			return nil
		}

		if info.ModTime().After(latestMod) {
			latestMod = info.ModTime()
			latestFile = p
		}
		return nil
	})

	if latestFile == "" {
		return "", time.Time{}, fmt.Errorf("No journal entry found")
	}

	return latestFile, latestMod, err
}
