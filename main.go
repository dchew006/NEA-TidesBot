// main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/go-rod/rod/lib/launcher"
)

const OutputImagePath = "tide_chart.png"

func main() {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is missing")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	// Regex explicitly tracking "tides [Month] [Day]" format style
	re := regexp.MustCompile(`(?i)(?:^|@\w+\s+)tides\s+([a-zA-Z]+)\s+(\d{1,2})$`)

	for update := range updates {
		if update.Message == nil || update.Message.Text == "" {
			continue
		}

		text := strings.TrimSpace(update.Message.Text)
		matches := re.FindStringSubmatch(text)

		// // debug
		// echodebug(bot, update.Message.Chat.ID, update.Message.MessageID, update.Message.Text)
		// log.Printf("Text: %s", text)
		// log.Printf("Matches: %v", matches)

		if len(matches) != 3 {
			continue
		}

		month := strings.Title(strings.ToLower(matches[1]))
		day := matches[2]

		err := orchestrateTidePipeline(bot, update.Message.Chat.ID, update.Message.MessageID, month, day)
		if err != nil {
			log.Printf("Pipeline failed for %s %s: %v", month, day, err)
			sendHelpFallback(bot, update.Message.Chat.ID, update.Message.MessageID)
		}
	}
}

func orchestrateTidePipeline(bot *tgbotapi.BotAPI, chatID int64, replyToID int, month, day string) error {

	// 1. Check if localized JSON source database tracking exists for the target month
	if _, err := os.Stat("tide_data.json"); os.IsNotExist(err) {
		log.Printf("Database cache missing. Launching tide-scraper for %s...", month)
		
		// CHANGED: Call the production binary instead of "go run"
		cmd := exec.Command("./tide-scraper", "--month", month)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("scraper step broke: %w", err)
		}
}

	// 2. Call function from graphing.go directly!
	// rawPrompt := fmt.Sprintf("%s %s", month, day)
	generatedHTMLFile, err := RenderChartForDate(month, day)
	if err != nil {
		return fmt.Errorf("graphing module execution failed: %w", err)
	}
	defer os.Remove(generatedHTMLFile) // Sweep temp html away post transaction

	// 3. Headless Screen Capture Pipeline execution
	err = captureChartSnapshot(generatedHTMLFile)
	if err != nil {
		return fmt.Errorf("visual render compilation failed: %w", err)
	}
	defer os.Remove(OutputImagePath)

	// 4. Dispatch Image back safely to group thread
	photoBytes, err := os.ReadFile(OutputImagePath)
	if err != nil {
		return err
	}

	photoFile := tgbotapi.FileBytes{Name: "tide_chart.png", Bytes: photoBytes}
	msg := tgbotapi.NewPhoto(chatID, photoFile)
	msg.ReplyToMessageID = replyToID
	msg.Caption = fmt.Sprintf("🌊 Singapore Tide Chart Timeline for %s %s", month, day)

	_, err = bot.Send(msg)
	return err
}

// func captureChartSnapshot(htmlPath string) error {

// 	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
// 	defer cancel()
// 	u := launcher.New().
// 		NoSandbox(true).           // <-- CRITICAL FOR GITHUB ACTIONS
// 		Headless(true)
		
// 	browser := rod.New().ControlURL(u.MustLaunch()).Context(ctx).MustConnect()
// 	defer browser.MustClose()

// 	absPath, _ := filepath.Abs(htmlPath)
// 	page := browser.MustPage("file://" + absPath).MustWaitLoad()

// 	// Snaps the high resolution DOM element matching our template architecture layout 
// 	el := page.MustElement("#dashboard")
// 	imgData, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)

// 	if err != nil {
// 		return err
// 	}

// 	return os.WriteFile(OutputImagePath, imgData, 0644)
// }

func captureChartSnapshot(htmlPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Grab the environment variable from your Dockerfile config
	binPath := os.Getenv("LAUNCHER_BIN")
	if binPath == "" {
		// Secure default fallback case if the env variable isn't found
		binPath = "/usr/bin/chromium" 
	}

	// 2. Initialize the launcher with the global container path
	u := launcher.New().
		Bin(binPath).        // <-- FORCES GO-ROD TO USE DOCKER'S INSTALLED CHROMIUM
		NoSandbox(true).     // <-- CRITICAL FOR GITHUB ACTIONS AND CLOUD CONTAINERS
		Headless(true)

	// 3. Mount control URLs using your custom timeout context block
	browser := rod.New().ControlURL(u.MustLaunch()).Context(ctx).MustConnect()
	defer browser.MustClose()

	absPath, _ := filepath.Abs(htmlPath)
	page := browser.MustPage("file://" + absPath).MustWaitLoad()

	// Snaps the high resolution DOM element matching our template architecture layout 
	el := page.MustElement("#dashboard")
	imgData, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
	if err != nil {
		return err
	}

	return os.WriteFile(OutputImagePath, imgData, 0644)
}

func sendHelpFallback(bot *tgbotapi.BotAPI, chatID int64, replyToID int) {
	text := "❌ *Failed to generate data charts.* \n\nPlease format your requested string exactly like this: \n`tides June 15`"

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyToMessageID = replyToID
	bot.Send(msg)
}

// echo debug function
func echodebug(bot *tgbotapi.BotAPI, chatID int64, replyToID int, text string) {
	text = fmt.Sprintf("DEBUG: `%s`", text)
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyToMessageID = replyToID

	if _, err := bot.Send(msg); err != nil {
		log.Printf("Failed to send user reply back to group: %v", err)
	}
}