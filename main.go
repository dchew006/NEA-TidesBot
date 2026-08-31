package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	log.Printf(" Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 120
	updates := bot.GetUpdatesChan(u)

	// Regex explicitly tracking "tides [Month] [Day]" format style
	re := regexp.MustCompile(`(?i)(?:^|@\w+\s+)tides\s+([a-zA-Z]+)\s+(\d{1,2})$`)

	for update := range updates {
		if update.Message == nil || update.Message.Text == "" {
			continue
		}

		chatID := update.Message.Chat.ID
		msgID := update.Message.MessageID
		text := strings.TrimSpace(update.Message.Text)

		log.Printf(" Received raw text: %q", text)

		matches := re.FindStringSubmatch(text)
		if len(matches) != 3 {
			log.Printf(" Regex no match. Ignoring message.")
			continue
		}

		rawMonth := matches[1]
		day := matches[2]

		// Normalize month to full name (July, not Jul)
		var parsedTime time.Time
		var err error
		if parsedTime, err = time.Parse("January", strings.ToLower(rawMonth)); err != nil {
			if parsedTime, err = time.Parse("Jan", strings.ToLower(rawMonth)); err != nil {
				log.Printf("Invalid month format: %s", rawMonth)
				continue
			}
		}
		
		month := strings.Title(parsedTime.Month().String())
		log.Printf(" Parsed -> Month: %s, Day: %s", month, day)

		err = orchestrateTidePipeline(bot, chatID, msgID, month, day)
		if err != nil {
			log.Printf("Pipeline error: %v", err)
			sendHelpFallback(bot, chatID, msgID)
		}
	}
}

func isMonthCached(filePath, requestedMonth string) bool {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf(" isMonthCached: File read error -> %v", err)
		return false
	}

	var allTides []DayTide
	if err := json.Unmarshal(fileData, &allTides); err != nil {
		log.Printf(" isMonthCached: JSON unmarshal error -> %v", err)
		return false
	}

	if len(allTides) == 0 {
		log.Printf(" isMonthCached: Empty array")
		return false
	}

	reqMonth := strings.ToLower(requestedMonth)
	for _, dayTide := range allTides {
		cleanDate := strings.TrimSpace(dayTide.Date)
		t, err := time.Parse("2006-01-02", cleanDate)
		if err == nil && strings.ToLower(t.Month().String()) == reqMonth {
			log.Printf(" isMonthCached: Found cached data for '%s'", requestedMonth)
			return true
		}
	}

	log.Printf(" isMonthCached: No cache found for '%s'", requestedMonth)
	return false
}

func orchestrateTidePipeline(bot *tgbotapi.BotAPI, chatID int64, replyToID int, month, day string) error {
	log.Printf("   Step 1: Checking data cache...")
	needsScraping := false

	if _, err := os.Stat("tide_data.json"); os.IsNotExist(err) {
		log.Printf(" Cache file missing. Launching scraper...")
		needsScraping = true
	} else if !isMonthCached("tide_data.json", month) {
		log.Printf(" Cache outdated or unreadable. Launching scraper...")
		needsScraping = true
	} else {
		log.Printf(" Cache valid. Skipping scraper.")
	}

	if needsScraping {
		if err := ScrapeTides(); err != nil {
			return fmt.Errorf("scraper failed: %w", err)
		}
		log.Printf("Scraper completed successfully.")
	}

	log.Printf("   Step 2: Rendering chart HTML...")
	generatedHTMLFile, err := RenderChartForDate(month, day)
	if err != nil {
		return fmt.Errorf("graphing failed: %w", err)
	}
	defer os.Remove(generatedHTMLFile)
	log.Printf("HTML rendered successfully.")

	log.Printf("   Step 3: Capturing screenshot...")
	err = captureChartSnapshot(generatedHTMLFile)
	if err != nil {
		return fmt.Errorf("screenshot failed: %w", err)
	}
	defer os.Remove(OutputImagePath)
	log.Printf("Screenshot captured.")

	log.Printf(" Step 4: Sending image to Telegram...")

	log.Printf(" Sending image to Telegram")

	photoFile := tgbotapi.FilePath(OutputImagePath)
	msg := tgbotapi.NewPhoto(chatID, photoFile)
	msg.ReplyToMessageID = replyToID
	msg.Caption = fmt.Sprintf("🌊 Singapore Tide Chart for %s %s", month, day)

	_, err = bot.Send(msg)
	if err != nil {
		return fmt.Errorf("failed to send photo: %w", err)
	}

	log.Printf("Pipeline complete for %s %s!", month, day)
	return nil
}

func captureChartSnapshot(htmlPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// run into Render memory issues so we need to use the pre-installed chromium in rod launcher
	launch := launcher.New().
		NoSandbox(true).
		Headless(true)

	binPath := os.Getenv("LAUNCHER_BIN") // DockerFile

	if binPath != "" {
		launch.Bin(binPath).
			Set("disable-dev-shm-usage").
			Set("disable-gpu").
			Set("single-process").
			Set("no-zygote").
			Set("renderer-process-limit", "1").
			Set("disable-software-rasterizer")
	} else {
		// debugging on mac
		if macPath, found := launcher.LookPath(); found {
			launch.Bin(macPath)
		}
	}

	launcherURL, err := launch.Launch()
	if err != nil {
		return fmt.Errorf("failed to launch chromium : %w", err)
	}

	browser := rod.New().ControlURL(launcherURL).Context(ctx)

	if err := browser.Connect(); err != nil {
		return fmt.Errorf("failed to connect to browser: %w", err)
	}
	defer browser.MustClose()

	absPath, err := filepath.Abs(htmlPath)
	if err != nil {
		return err
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: "file://" + absPath})
	if err != nil {
		return err
	}
	defer page.Close()

	page.MustWaitLoad()

	el, err := page.Element("#dashboard")
	if err != nil {
		return err
	}

	imgData, err := el.Screenshot(proto.PageCaptureScreenshotFormatPng, 100)
	if err != nil {
		return err
	}

	return os.WriteFile(OutputImagePath, imgData, 0644)
}

func sendHelpFallback(bot *tgbotapi.BotAPI, chatID int64, replyToID int) {
	text := "Failed to generate data charts.\n\nPlease format your request exactly like this:\n`tides June 15`"
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyToMessageID = replyToID
	bot.Send(msg)
}