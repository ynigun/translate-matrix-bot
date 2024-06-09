package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"github.com/joho/godotenv"
	"github.com/ynigun/translate-matrix-bot/anthropic"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

var (
	matrixClient      *mautrix.Client
	AnthropicURL      = "https://api.anthropic.com/v1/messages"
	AnthropicAPIModel = "claude-3-haiku-20240307"
)

func main() {
	// Initialize logger
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	// Initialize Matrix client
	matrixClient, err := mautrix.NewClient(os.Getenv("MATRIX_SERVER"), "", "")
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
	// Set up event handler
	log.Println("Setting up event handler...")

	syncer := matrixClient.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, func(ctx context.Context, evt *event.Event) {

		if evt.Sender != matrixClient.UserID {
			message := evt.Content.AsMessage()
			if message.MsgType == event.MsgText {
				text := message.Body
				if text != "" {
					matrixClient.SendReceipt(ctx, evt.RoomID, evt.ID, event.ReceiptTypeRead, nil)
					matrixClient.UserTyping(ctx, evt.RoomID, true, 5*time.Second)
					translatedText, err := translateMessage(ctx, text)
					if err != nil {
						log.Printf("Error processing message: %v", err)
						matrixClient.SendNotice(ctx, evt.RoomID, "שגיאה במהלך התרגום")
					} else {
						log.Printf("Sending translated message: %s", translatedText)
						//matrixClient.SendText(ctx, evt.RoomID, translatedText)
						content := event.MessageEventContent{
							MsgType: event.MsgText,
							Body:    translatedText,
							RelatesTo: &event.RelatesTo{
								InReplyTo: &event.InReplyTo{
									//Type:    event.RelReply,
									EventID: evt.ID,
								},
							},
						}
						matrixClient.SendMessageEvent(ctx, evt.RoomID, event.EventMessage, content)

					}
					matrixClient.UserTyping(ctx, evt.RoomID, false, 0)
				}
			} else {
				log.Println("Received message with media content")
				matrixClient.SendNotice(ctx, evt.RoomID, "ניתן לתרגם רק הודעות טקסט")
			}
		}
	})
	syncer.OnEventType(event.StateMember, func(ctx context.Context, evt *event.Event) {
		if membership := evt.Content.AsMember().Membership; membership == event.MembershipInvite && id.UserID(evt.GetStateKey()) == matrixClient.UserID {
			log.Printf("Received invite for room: %s", evt.RoomID)
			matrixClient.JoinRoomByID(ctx, evt.RoomID)
			log.Printf("Joined room: %s", evt.RoomID)
		}
	})
	// Start sync loop
	log.Println("Bot started!")
	err = matrixClient.Sync()
	if err != nil {
		log.Fatalf("Sync() returned error: %v", err)
	}

	// Access the joined rooms after the initial sync
}

type TranslationRequest struct {
	Model       string `json:"model"`
	MaxTokens   int    `json:"max_tokens"`
	Temperature int    `json:"temperature"`
	Prompt      string `json:"prompt"`
}

type TranslationResponse struct {
	Lang string `json:"lang"`
	Text string `json:"text"`
}

func translateMessage(ctx context.Context, text string) (string, error) {
	client := &anthropic.Client{
		APIKey:           os.Getenv("ANTHROPIC_API_KEY"),
		AnthropicVersion: "2023-06-01",
		HTTP:             &http.Client{},
	}

	prompt := fmt.Sprintf(`תרגם את הטקסט לעברית

שמור על דיוק בתרגום
אם יש ספק הצמד למקור ואל תשנה את המשמעות כדי שישמע טוב בעברית יומיומית
עם זאת במקרה שהתרגום לא משנה מהמשמעות כדאי שהתרגום יהיה בעברית יומיומית

לא מדובר בתרגום חופשי או ספרותי אלא בתרגום טכני שנועד בעיקר להעביר את התוכן והמשמעות המדויקים

אני אשלח את הטקסט בלי JSON
לדוגמה
Важливе повідомлення: зміст повідомлення

ואתה תחזיר לי JSON עם התרגום לעברית
{"lang":"he","text":"הודעה חשובה: תוכן הודעה"}

דוגמה נוספת

קלט:

ray AB cafe вже в усіх швидкісних поїздах Інтерсіті+

Щойно скуштуєте нове меню — приймаємо зворотний зв'язок всіма каналами: Viber, Telegram, Apple Messages, Facebook Messenger, у застосунку Укрзалізниці та через QR-коди безпосередньо у вагонах.

Смачних та комфортних подорожей💙

פלט:
{
"lang": "he",
"text": "בית הקפה ray AB כבר נמצא בכל רכבות האינטרסיטי+ המהירות
ברגע שתטעמו את התפריט החדש - אנו מקבלים משוב בכל הערוצים: Viber, Telegram, Apple Messages, Facebook Messenger, באפליקציה של Ukrzaliznytsia ודרך קודי QR ישירות בקרונות.
נסיעות טעימות ונוחות💙"
}

קלט:
🔹#عاجل وسائل إعلام يمنيّة:
مصدر أمني ، الإعلان غداً عن إنجاز أمني استراتيجي كبير وغير مسبوق..

פלט:
{
"lang": "he",
"text": "🔹#דחוף כלי תקשורת תימניים:
מקור ביטחוני, מחר יוכרז על הישג ביטחוני אסטרטגי גדול וחסר תקדים.."
}

תזכור שצריך לתרגם דווקא לעברית lang=he

לא לתרגם לערבית או לאוקראינית`)
	req := &anthropic.MessageRequest{
		Model:     AnthropicAPIModel,
		MaxTokens: 1024,
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
	prefix := `{
"lang": "he",
"text": "`
	if strings.HasPrefix(translatedText, prefix) && strings.HasSuffix(translatedText, `"
}`) {
		translatedText = strings.TrimPrefix(translatedText, prefix)
		translatedText = strings.TrimSuffix(translatedText, `"
}`)
		return translatedText, nil
	}

	var translatedData RawText
	if err := json.Unmarshal([]byte(translatedText), &translatedData); err != nil {
		log.Printf("Error decoding JSON: %v", err)
		return translatedText, nil
	}

	if translatedData.Lang != "he" {
		return "", fmt.Errorf("translated language is not Hebrew (he)")
	}
	return translatedData.Text, nil
}

type RawText struct {
	Lang string `json:"lang"`
	Text string `json:"text"`
}

func (rt *RawText) UnmarshalJSON(data []byte) error {
	var rawJSON map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawJSON); err != nil {
		return err
	}

	if err := json.Unmarshal(rawJSON["lang"], &rt.Lang); err != nil {
		return err
	}

	textBytes, err := rawJSON["text"].MarshalJSON()
	if err != nil {
		return err
	}

	var textStr string
	if err := json.Unmarshal(textBytes, &textStr); err != nil {
		textStr = string(rawJSON["text"])
	}
	rt.Text = textStr

	return nil
}
