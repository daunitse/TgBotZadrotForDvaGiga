package main

import (
	"flag"
	"fmt"
	"github.com/mymmrac/telego"
	"strconv"
	"sync"

	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tu "github.com/mymmrac/telego/telegoutil"
)

var tokenFile = flag.String("token", ".token", "Telegram token")

var mapMutex = &sync.RWMutex{}

var MStatus = make(map[int64]int)

var MatchStatus = make(map[string]string)

var StartTimeStatus = make(map[string]int)

var EndTimeStatus = make(map[string]int)

var Usernames = make(map[int64]string)

func main() {
	flag.Parse()

	botToken, err := filepath.Abs(*tokenFile)
	fatalOnError(err)

	t, err := os.ReadFile(botToken)
	fatalOnError(err)

	bot, err := telego.NewBot(strings.TrimSpace(string(t)), telego.WithDefaultDebugLogger())
	fatalOnError(err)

	botUser, err := bot.GetMe()
	fatalOnError(err)
	fmt.Printf("Bot %+v\n", botUser)

	updates, err := bot.UpdatesViaLongPolling(nil)
	fatalOnError(err)
	defer bot.StopLongPolling()

	db, err := newDb("app.db")
	fatalOnError(err)
	defer func() {
		_ = db.Close()
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	stopChan := make(chan struct{})
	go deleteBucketEveryNoon(db, stopChan)

	err = db.GiveUserRoles(daunitseID, adminRole)
	notFatalButError(err)
	err = db.GiveUserRoles(carpawellID, adminRole)
	notFatalButError(err)

	for {
		select {
		case sig := <-sigChan:
			log.Printf("Received %v signal", sig)
			close(stopChan)

			return
		case upd := <-updates:
			handleUpdate(upd, db, bot)
		}
	}

}

func handleUpdate(upd telego.Update, db *database, bot *telego.Bot) {
	if upd.Message == nil {
		log.Printf("Received empty message: %#v", upd.Message)
		return
	}

	userID := upd.Message.From.ID

	chatID := upd.Message.Chat.ID

	var groupID int64 = groupID

	m := strings.ToLower(upd.Message.Text)

	role, err := db.CheckUserRole(userID)
	if err != nil {
		notFatalButError(err)
		return
	}

	if chatID == groupID && role != adminRole {
		err = db.GiveUserRoles(userID, userRole)
		if err != nil {
			notFatalButError(err)
			return
		}
		role = userRole
	}

	if strings.Contains(m, " чс A?") {
		sendMessageIfCheckErrorNoNeed(bot, chatID, "Нахуй спрашиваешь? Тыкни на меня и узнаешь")

		return
	}

	if chatID == groupID {

		return
	}

	mapMutex.Lock()
	defer mapMutex.Unlock()

	_, exists := MStatus[userID]

	if !exists {
		MStatus[userID] = 0
	}

	switch {
	case MStatus[userID] == 0:

		switch {
		case upd.Message.Chat.Type == privateCHat && role == bomzRole:
			unknownUserMessageCase(bot, upd, chatID)
			return
		case m == "/start":
			sendBaseKeyboardMessage(bot, chatID, "Тыкай кнопки, не забудь что в 12 дня твой статус обновится")
			return
		case m == "хочу играть":
			sendTimeSelectionKeyboard(bot, chatID, "Выбери время во сколько хочешь начать", 0)
			MStatus[userID] = 1
			return
		case m == "не хочу играть":
			dontWannaPlay(bot, chatID)

		case m == "узнать кто играет":
			whoPlay(bot, chatID)
		default:
			sendBaseKeyboardMessage(bot, chatID, "Тыкай кнопки, не забудь что в 12 дня твой статус обновится")
			return

		}

	case MStatus[userID] == 1:
		switch m {
		case "назад":
			sendBaseKeyboardMessage(bot, chatID, "Тыкай кнопки")
			MStatus[userID] = 0
			return
		case "12ч", "13ч", "14ч", "15ч",
			"16ч", "17ч", "18ч", "19ч",
			"20ч", "21ч", "22ч", "23ч",
			"0ч", "1ч", "2ч", "3ч",
			"4ч", "5ч", "6ч", "7ч",
			"8ч", "9ч", "10ч", "11ч":
			startTimePlay(bot, chatID, m)

		default:
			return

		}

	case MStatus[userID] == 2:
		switch m {
		case "назад":
			sendMessageIfCheckErrorNoNeed(bot, chatID, "Ну ок, измени время начала")
			MStatus[userID] = 1
			return

		case "12ч", "13ч", "14ч", "15ч",
			"16ч", "17ч", "18ч", "19ч",
			"20ч", "21ч", "22ч", "23ч",
			"0ч", "1ч", "2ч", "3ч",
			"4ч", "5ч", "6ч", "7ч",
			"8ч", "9ч", "10ч", "11ч":
			b := endTimePlay(bot, chatID, m)
			if b == true {
				match(bot, chatID)
			}
			return

		}

	}

}

func dontWannaPlay(bot *telego.Bot, userID int64) {
	_, exists := EndTimeStatus[Usernames[userID]]
	if exists {
		delete(EndTimeStatus, Usernames[userID])
		delete(StartTimeStatus, Usernames[userID])
		sendMessageIfCheckErrorNoNeed(bot, userID, "Удалил тебя, пидорасик")

		for username, startTime := range StartTimeStatus {
			_, exists := EndTimeStatus[username]
			if exists && username != Usernames[userID] {

				start := maximum(startTime, StartTimeStatus[Usernames[userID]])
				end := minimum(EndTimeStatus[Usernames[userID]], EndTimeStatus[username])
				if start < end {
					sendMessageIfCheckErrorNoNeed(bot, groupID, "Просто сообщаю, что этот пидор "+Usernames[userID]+" больше не хочет играть")
					whoPlay(bot, groupID)

					return
				}

			}
		}

		return
	}
	sendMessageIfCheckErrorNoNeed(bot, userID, "Ты и так не хотел, долбоеб")

	return
}

func whoPlay(bot *telego.Bot, userID int64) {
	msg := "Сегодня играет: \n"

	for user, startTime := range StartTimeStatus {
		_, exists := Usernames[userID]
		if exists {
			if user == Usernames[userID] {
				continue
			}
		}
		startTimeString := timeIntToTimeString(startTime)
		_, exists = EndTimeStatus[user]
		if !exists {
			continue
		}
		endTimeString := timeIntToTimeString(EndTimeStatus[user])
		msg = msg + user + " в " + startTimeString + "ч закончит в " + endTimeString + "ч\n"

	}
	if msg == "Сегодня играет: \n" {
		msg = "Сегодня, пока, никто не играет"
	}
	sendMessageIfCheckErrorNoNeed(bot, userID, msg)
}

func match(bot *telego.Bot, userID int64) {

	_, exists := StartTimeStatus[Usernames[userID]]

	if !exists {
		sendBaseKeyboardMessage(bot, userID, "Что-то пошло не так, попробуй позже")
		MStatus[userID] = 0

		return
	}

	_, exists = EndTimeStatus[Usernames[userID]]

	if !exists {
		sendBaseKeyboardMessage(bot, userID, "Что-то пошло не так, попробуй позже")
		MStatus[userID] = 0

		return
	}

	msg := "Случилось чудо, у нас мэтч \n" + Usernames[userID] + " " + timeIntToTimeString(StartTimeStatus[Usernames[userID]]) + "ч-" + timeIntToTimeString(EndTimeStatus[Usernames[userID]]) + "ч\n"

	for username, startTime := range StartTimeStatus {
		_, exists := EndTimeStatus[username]
		if exists && username != Usernames[userID] {

			start := maximum(startTime, StartTimeStatus[Usernames[userID]])
			end := minimum(EndTimeStatus[Usernames[userID]], EndTimeStatus[username])
			if start < end {
				msg = msg + username + " " + timeIntToTimeString(startTime) + "ч-" + timeIntToTimeString(EndTimeStatus[username]) + "ч\n"
			}

		}
	}
	if msg != "Случилось чудо, у нас мэтч \n"+Usernames[userID]+" "+timeIntToTimeString(StartTimeStatus[Usernames[userID]])+"ч-"+timeIntToTimeString(EndTimeStatus[Usernames[userID]])+"ч\n" {
		sendMessageIfCheckErrorNoNeed(bot, groupID, msg)
	}
	return

}

func reminder(bot *telego.Bot, userID int64, targetTime time.Time) { //todo сделать нормально, чтобы функция переставала работать
	//todo если ее вызывают второй раз и не работала, если нет сопартийцев
	reminderTime := targetTime.Add(-1 * time.Hour)

	fmt.Println(reminderTime)
	fmt.Println(timeIntToTimeString(StartTimeStatus[Usernames[userID]]))
	fmt.Println(targetTime.Format("15"))

	if reminderTime.After(time.Now()) {
		time.Sleep(time.Until(reminderTime))

		if timeIntToTimeString(StartTimeStatus[Usernames[userID]]) == targetTime.Format("15") || "0"+timeIntToTimeString(StartTimeStatus[Usernames[userID]]) == targetTime.Format("15") {
			err := sendMessage(bot, userID, "Напоминаю, что ты хотел играть через час")
			if err != nil {
				notFatalButError(err)
				return
			}
			whoPlay(bot, userID)
		}
	}

	return
}

func getChatIDForUserID(username string) (chatID int64) {
	for chatID, user := range Usernames {
		if user == username {
			return chatID
		}
	}
	return 0
}

func timeIntToTimeString(time int) string {
	timeString := strconv.Itoa(time + 12)
	if time+12 > 23 {
		timeString = strconv.Itoa(time - 12)
	}
	return timeString
}

func removeDuplicates(input string) string {
	seen := make(map[string]bool)
	result := []string{}

	for _, name := range strings.Fields(input) {
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}

	return strings.Join(result, " ")
}

func maximum(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minimum(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func endTimePlay(bot *telego.Bot, userID int64, hour string) bool {

	hourInt, err := strconv.Atoi(strings.TrimSuffix(hour, "ч"))
	if err != nil {
		notFatalButError(err)
		return false
	}

	_, exist := Usernames[userID]

	if !exist {
		username, err := getUsernameFromUserID(bot, userID, groupID)
		if err != nil {
			notFatalButError(err)
			return false
		}
		Usernames[userID] = username
	}

	hourForStatus := hourInt + 12
	if hourForStatus > 23 {
		hourForStatus = hourInt - 12
	}

	_, exist = StartTimeStatus[Usernames[userID]]

	if !exist {
		sendMessageIfCheckErrorNoNeed(bot, userID, "что-то пошло не так, попробуй позже")
		MStatus[userID] = 0

		return false
	}

	if StartTimeStatus[Usernames[userID]] > hourForStatus {
		sendBaseKeyboardMessage(bot, userID, "Сломал время, ты долбоеб? Нормально проставь время")
		MStatus[userID] = 0

		return false
	}

	EndTimeStatus[Usernames[userID]] = hourForStatus

	sendBaseKeyboardMessage(bot, userID, "Cупер, записал тебя")

	MStatus[userID] = 0

	return true

}

func startTimePlay(bot *telego.Bot, userID int64, hour string) { //need help. Надо придумать как сделать так,
	//чтобы не появлялось часть кнопок.

	hourInt, err := strconv.Atoi(strings.TrimSuffix(hour, "ч"))
	if err != nil {
		notFatalButError(err)
		return
	}
	_, exist := Usernames[userID]

	if !exist {
		username, err := getUsernameFromUserID(bot, userID, groupID)
		if err != nil {
			notFatalButError(err)
			return
		}
		Usernames[userID] = username
	}

	hourForStatus := hourInt + 12
	if hourForStatus > 23 {
		hourForStatus = hourInt - 12
	}

	StartTimeStatus[Usernames[userID]] = hourForStatus

	_, exist = MStatus[userID]

	if !exist {
		sendMessageIfCheckErrorNoNeed(bot, userID, "Что-то пошло не так, попробуй позже")
		MStatus[userID] = 0
		return
	}

	sendTimeSelectionKeyboard(bot, userID, "Теперь выбери во сколько хочешь закончить играть", hourForStatus)
	MStatus[userID] = 2

	now := time.Now()
	targetTime := time.Date(now.Year(), now.Month(), now.Day(), hourInt, 0, 0, 0, now.Location())
	if hourInt >= 0 && hourInt <= 11 {
		tomorrowDay := now.Day() + 1
		targetTime = time.Date(now.Year(), now.Month(), tomorrowDay, hourInt, 0, 0, 0, now.Location())
	}
	go reminder(bot, userID, targetTime)

	return
}

func getUsernameFromUserID(bot *telego.Bot, userID int64, chatID int64) (string, error) {
	prm := &telego.GetChatMemberParams{
		ChatID: telego.ChatID{
			ID: chatID,
		},
		UserID: userID,
	}

	chatMember, err := bot.GetChatMember(prm)
	if err != nil {
		notFatalButError(err)
		return "", err
	}

	user := chatMember.MemberUser()

	if user.Username != "" {
		return "@" + user.Username, nil
	}
	name := user.FirstName + " " + user.LastName
	if name == " " {
		return strconv.FormatInt(userID, 10), nil
	}
	return name, nil
}

func sendBaseKeyboardMessage(bot *telego.Bot, ChatID int64, text string) {
	keyboard := tu.Keyboard(
		tu.KeyboardRow(
			tu.KeyboardButton("Хочу играть"),
			tu.KeyboardButton("Не хочу играть"),
			tu.KeyboardButton("Узнать кто играет"),
		),
	).WithResizeKeyboard()

	msg := tu.Message(
		tu.ID(ChatID),
		text,
	).WithReplyMarkup(keyboard)

	_, err := bot.SendMessage(msg)
	if err != nil {
		notFatalButError(err)
		return
	}
}

func sendTimeSelectionKeyboard(bot *telego.Bot, ChatID int64, text string, startTime int) {

	keyboard := tu.Keyboard(

		tu.KeyboardRow(
			tu.KeyboardButton("12ч"),
			tu.KeyboardButton("13ч"),
			tu.KeyboardButton("14ч"),
			tu.KeyboardButton("15ч"),
		),
		tu.KeyboardRow(
			tu.KeyboardButton("16ч"),
			tu.KeyboardButton("17ч"),
			tu.KeyboardButton("18ч"),
			tu.KeyboardButton("19ч"),
		),
		tu.KeyboardRow(
			tu.KeyboardButton("20ч"),
			tu.KeyboardButton("21ч"),
			tu.KeyboardButton("22ч"),
			tu.KeyboardButton("23ч"),
		),
		tu.KeyboardRow(
			tu.KeyboardButton("0ч"),
			tu.KeyboardButton("1ч"),
			tu.KeyboardButton("2ч"),
			tu.KeyboardButton("3ч"),
		),
		tu.KeyboardRow(
			tu.KeyboardButton("4ч"),
			tu.KeyboardButton("5ч"),
			tu.KeyboardButton("6ч"),
			tu.KeyboardButton("7ч"),
		),
		tu.KeyboardRow(
			tu.KeyboardButton("8ч"),
			tu.KeyboardButton("9ч"),
			tu.KeyboardButton("10ч"),
			tu.KeyboardButton("11ч"),
		),
		tu.KeyboardRow(
			tu.KeyboardButton("Назад"),
		),
	).WithResizeKeyboard()

	msg := tu.Message(
		tu.ID(ChatID),
		text,
	).WithReplyMarkup(keyboard)

	_, err := bot.SendMessage(msg)
	if err != nil {
		notFatalButError(err)
		return
	}
}

func unknownUserMessageCase(bot *telego.Bot, upd telego.Update, chatID int64) {
	err := sendMessage(bot, chatID, "Hi, if u want to use our cute bot - write @daunitse")
	if err != nil {
		notFatalButError(err)
		return
	}
	sendMessageIfCheckErrorNoNeed(bot, daunitseID, "Он со мной связался\n"+upd.Message.From.FirstName+" @"+upd.Message.From.Username)
}

func sendMessageIfCheckErrorNoNeed(bot *telego.Bot, ChatID int64, text string) {
	msg := tu.Message(
		tu.ID(ChatID),
		text,
	)

	_, err := bot.SendMessage(msg)
	if err != nil {
		notFatalButError(err)
		return
	}
}

func writeErrorToFile(err error) error {

	file, openErr := os.OpenFile("errors.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, perm)
	if openErr != nil {
		return fmt.Errorf("не удалось открыть файл: %v", openErr)
	}
	defer func() {
		_ = file.Close()
	}()

	logger := log.New(file, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)

	logger.Println(err)

	return nil
}

func deleteBucketEveryNoon(db *database, stopChan chan struct{}) {
	const noonHour, minuteInHour = 12, 60

	noonCh := make(chan bool)

	go func() {
		for {
			t := time.NewTimer(time.Minute * time.Duration(minuteInHour-time.Now().Minute()))
			select {
			case <-stopChan:
				t.Stop()
				return
			case <-t.C:
				t.Reset(time.Minute * time.Duration(minuteInHour-time.Now().Minute()))

				if time.Now().Hour() == noonHour {
					noonCh <- true
				}
			}
		}
	}()

	for {
		select {
		case <-stopChan:
			return
		case <-noonCh:
		}
		err := db.ResetBucket(playersTodayBucket)
		MStatus = make(map[int64]int)
		StartTimeStatus = make(map[string]int)
		EndTimeStatus = make(map[string]int)
		if err != nil {
			notFatalButError(err)
			continue
		}

	}

}

func sendMessage(bot *telego.Bot, ChatID int64, text string) error {
	msg := tu.Message(
		tu.ID(ChatID),
		text,
	)

	_, err := bot.SendMessage(msg)
	if err != nil {
		log.Printf("Не смог ответить %d пользователю на сообщение '%s': %s", ChatID, text, err)
		return err
	}
	return nil
}

func fatalOnError(err error) {
	if err != nil {
		log.Printf("fatal error: %s", err)
		os.Exit(1)
	}
}

func notFatalButError(err error) {
	if err != nil {
		log.Printf("error: %s", err)
		err = writeErrorToFile(err)
		if err != nil {
			log.Printf("Ну все, пиздец")
		}
	}

	return
}
