package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
	"github.com/joho/godotenv"
	"github.com/ynigun/translate-matrix-bot/anthropic"
	_ "github.com/mattn/go-sqlite3"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	matrixClient      *mautrix.Client
	AnthropicURL      = "https://api.anthropic.com/v1/messages"
	AnthropicAPIModel = "claude-3-haiku-20240307"
	db                *sql.DB
	adminUserID       id.UserID
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	matrixClient, err = mautrix.NewClient(os.Getenv("MATRIX_SERVER"), "", "")
	if err != nil {
		log.Fatalf("Error initializing Matrix client: %v", err)
	}
	matrixClient.AccessToken = os.Getenv("MATRIX_ACCESS_TOKEN")
	matrixClient.UserID = id.UserID(os.Getenv("MATRIX_USER_ID"))

	eventType := "com.example.translatebot.store"
	store := mautrix.NewAccountDataStore(eventType, matrixClient)
	matrixClient.Store = store
	filter := mautrix.Filter{
		AccountData: mautrix.FilterPart{
			Limit: 20,
			NotTypes: []event.Type{
				event.NewEventType(eventType),
			},
		},
	}
	matrixClient.Syncer.(*mautrix.DefaultSyncer).FilterJSON = &filter

	db, err = sql.Open("sqlite3", "filter.db")
	if err != nil {
		log.Fatalf("Error opening SQLite database: %v", err)
	}
	defer db.Close()

	if err := createFilterTable(); err != nil {
		log.Fatalf("Error creating filter table: %v", err)
	}

	adminUserID = id.UserID(os.Getenv("ADMIN_USER_ID"))

	log.Println("Setting up event handler...")

	syncer := matrixClient.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, handleMessage)
	syncer.OnEventType(event.StateMember, handleMembership)

	log.Println("Bot started!")
	err = matrixClient.Sync()
	if err != nil {
		log.Fatalf("Sync() returned error: %v", err)
	}
}

func createFilterTable() error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS filter_keywords (keyword TEXT PRIMARY KEY)`)
	return err
}

func addFilterKeyword(keyword string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO filter_keywords (keyword) VALUES (?)`, keyword)
	return err
}

func removeFilterKeyword(keyword string) error {
	_, err := db.Exec(`DELETE FROM filter_keywords WHERE keyword = ?`, keyword)
	return err
}

func getFilterKeywords() ([]string, error) {
	rows, err := db.Query(`SELECT keyword FROM filter_keywords`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keywords []string
	for rows.Next() {
		var keyword string
		if err := rows.Scan(&keyword); err != nil {
			return nil, err
		}
		keywords = append(keywords, keyword)
	}
	return keywords, nil
}


func handleMessage(ctx context.Context, evt *event.Event) {
	if evt.Sender != matrixClient.UserID {
		message := evt.Content.AsMessage()
		if message.MsgType == event.MsgText {
			text := message.Body
			if text != "" {
				if evt.Sender == adminUserID {
					// פקודות ממשתמש מנהל
					if strings.HasPrefix(text, "!add ") {
						keyword := strings.TrimPrefix(text, "!add ")
						if err := addFilterKeyword(keyword); err != nil {
							log.Printf("Error adding filter keyword: %v", err)
						} else {
							matrixClient.SendNotice(ctx, evt.RoomID, fmt.Sprintf("Added filter keyword: %s", keyword))
						}
						return
					} else if strings.HasPrefix(text, "!remove ") {
						keyword := strings.TrimPrefix(text, "!remove ")
						if err := removeFilterKeyword(keyword); err != nil {
							log.Printf("Error removing filter keyword: %v", err)
						} else {
							matrixClient.SendNotice(ctx, evt.RoomID, fmt.Sprintf("Removed filter keyword: %s", keyword))
						}
						return
					}
				}
				
				keywords, err := getFilterKeywords()
				if err != nil {
					log.Printf("Error getting filter keywords: %v", err)
				} else {
					shouldFilter := false
					var matchedKeyword string
					for _, keyword := range keywords {
						matched, _ := regexp.MatchString(keyword, text)
						if matched {
							shouldFilter = true
							matchedKeyword = keyword
							break
						}
					}
					if shouldFilter {
						matrixClient.SendNotice(ctx, evt.RoomID, fmt.Sprintf("ההודעה שלך נחסמה מכיוון שהיא מכילה את הביטוי: %s", matchedKeyword))
					} else {
						matrixClient.SendReceipt(ctx, evt.RoomID, evt.ID, event.ReceiptTypeRead, nil)
						matrixClient.UserTyping(ctx, evt.RoomID, true, 5*time.Second)
						

						// Remove emojis from the entire text
						textWithoutEmojis := removeEmojis(text)
						
						// Remove signature after removing emojis
						textWithoutSignature := removeSignature(textWithoutEmojis)
						
						translatedText, err := translateMessage(ctx, textWithoutSignature)
						if err != nil {
							log.Printf("Error processing message: %v", err)
							matrixClient.SendNotice(ctx, evt.RoomID, "שגיאה במהלך התרגום")
						} else {
							log.Printf("Sending translated message: %s", translatedText)
							content := event.MessageEventContent{
								MsgType: event.MsgText,
								Body:    translatedText,
								RelatesTo: &event.RelatesTo{
									InReplyTo: &event.InReplyTo{
										EventID: evt.ID,
									},
								},
							}
							matrixClient.SendMessageEvent(ctx, evt.RoomID, event.EventMessage, content)
						}
						matrixClient.UserTyping(ctx, evt.RoomID, false, 0)
					}
				}
			}
		} else {
			log.Println("Received message with media content")
			matrixClient.SendNotice(ctx, evt.RoomID, "ניתן לתרגם רק הודעות טקסט")
		}
	}
}

func handleMembership(ctx context.Context, evt *event.Event) {
	if membership := evt.Content.AsMember().Membership; membership == event.MembershipInvite && id.UserID(evt.GetStateKey()) == matrixClient.UserID {
		log.Printf("Received invite for room: %s", evt.RoomID)
		matrixClient.JoinRoomByID(ctx, evt.RoomID)
		log.Printf("Joined room: %s", evt.RoomID)
	}
}

func removeEmojis(text string) string {
	emojiRegex := regexp.MustCompile(`[\p{So}\p{Sk}]`)
	return emojiRegex.ReplaceAllString(text, "")
}

func removeSignature(text string) string {
	// Split the text into lines
	lines := strings.Split(strings.TrimSpace(text), "\n")

	// If there's only one line, return it as is
	if len(lines) <= 1 {
		return text
	}

	// Check if the last line looks like a signature
	lastLine := strings.TrimSpace(lines[len(lines)-1])
	
	// Define patterns for signatures
	signaturePatterns := []string{
		`^-\s.*`,                    // Lines starting with "- "
		`^[—–]\s.*`,                 // Lines starting with "— " or "– "
		`^[~\*]\s.*`,                // Lines starting with "~ " or "* "
		`^[\p{L}\p{N}]+:?\s.*`,      // Lines starting with a name followed by ": " or just " "
		`^[•✦★☆◆◇■□●○]\s.*`,        // Lines starting with various bullet points
		`^[\p{So}\p{Sk}]+.*`,        // Lines starting with special characters or symbols
		`^.*\s?[\p{So}\p{Sk}]+$`,    // Lines ending with special characters or symbols
		`^.*™$`,                     // Lines ending with ™
	}

	isSignature := false
	for _, pattern := range signaturePatterns {
		if matched, _ := regexp.MatchString(pattern, lastLine); matched {
			isSignature = true
			break
		}
	}

	// Additional check for lines that are mostly non-letter characters
	if !isSignature {
		nonLetterCount := 0
		for _, r := range lastLine {
			if !unicode.IsLetter(r) {
				nonLetterCount++
			}
		}
		if float64(nonLetterCount)/float64(len(lastLine)) > 0.5 {
			isSignature = true
		}
	}

	if isSignature {
		// Remove the last line if it matches a signature pattern
		return strings.TrimSpace(strings.Join(lines[:len(lines)-1], "\n"))
	}

	return text
}
func translateMessage(ctx context.Context, text string) (string, error) {
	client := &anthropic.Client{
		APIKey:           os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicVersion: "2023-06-01",
		HTTP:             &http.Client{},
	}

	prompt := fmt.Sprintf(`
אתה בוט מתרגם המתמחה בתרגום הודעות טלגרם מערבית או אוקראינית לעברית. עליך לפעול לפי ההנחיות הבאות:

1. תרגם את ההודעה במדויק מערבית או אוקראינית לעברית.
2. התחל את התרגום מיד, ללא כותרת או הקדמה.
3. שמור על המשמעות והטון המקוריים של ההודעה.
4. אל תוסיף פרשנות, הערות או שיפוט מוסרי לתוכן.
5. אם ההודעה מכילה תוכן אלים או בוטה, תרגם אותו כמות שהוא ללא צנזורה או ריכוך.
6. אם יש מונחים או ביטויים ייחודיים לתרבות המקור, תרגם אותם ככל האפשר והוסף הסבר קצר בסוגריים אם נדרש.
7. שמור על מבנה ההודעה המקורי, כולל פסקאות, רשימות וכו'.

דוגמה לפורמט התגובה:

```
[כאן יבוא התרגום המלא של ההודעה]
```

זכור: המטרה היא לספק תרגום מדויק ואובייקטיבי, ללא שום תוספות או השמטות, ולהתחיל את התרגום מיד ללא כותרת.`)

	req := &anthropic.MessageRequest{
		Model:     AnthropicAPIModel,
		MaxTokens: len([]rune(text)),
		System:    prompt,
		Messages: []anthropic.Message{
			{
				Role:    "user",
				Content: []interface{}{map[string]string{"type": "text", "text": text}},
			},
		},
	}

	resp, err := client.CreateMessage(req)
	if err != nil {
		return "", fmt.Errorf("error calling Anthropic API: %v", err)
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("empty response from Anthropic API")
	}

	translatedText := resp.Content[0].(map[string]interface{})["text"].(string)

	return translatedText, nil
}
