package main

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/fatih/color"
	"golang.org/x/crypto/ssh"
)

var (
	clearScreen = "\033[H\033[2J"
	hideCursor  = "\033[?25l"

	wallColor      = color.New(color.FgCyan)
	verticalWall   = "║"
	horizontalWall = "═"
	topLeft        = "╔"
	topRight       = "╗"
	bottomRight    = "╝"
	bottomLeft     = "╚"

	empty = " "
)

// PlayerSession handles all user session data
type PlayerSession struct {
	mux     sync.Mutex
	channel ssh.Channel
	name    string
	answers map[AnswerKey]string
	screen  *PlayerScreen
}

func (s *PlayerSession) UpdateWindows(rows, columns int) {
	s.mux.Lock()
	defer s.mux.Unlock()

	s.screen = NewPlayerScreen(rows, columns)
	s.screen.Render()
}

type PlayerScreen struct {
	Columns int
	Rows    int
	Tiles   [][]string
}

func NewPlayerScreen(rows, columns int) *PlayerScreen {
	rowArray := make([][]string, rows)
	for i := 0; i < rows; i++ {
		rowArray[i] = make([]string, columns)
		for j := 0; j < columns; j++ {
			rowArray[i][j] = empty
		}
	}
	return &PlayerScreen{Columns: columns, Rows: rows, Tiles: rowArray}
}

func (s *PlayerScreen) Render() string {

	var buff bytes.Buffer
	for i := 0; i < s.Rows; i++ {
		for j := 0; j < s.Columns; j++ {
			buff.WriteString(s.Tiles[i][j])
		}
		buff.WriteString("\r\n")
	}
	return buff.String()
}

func (s *PlayerScreen) Compute(quizz *Quizz) {

	// Questions Board
	s.DrawRect(0, 0, 55, s.Rows-1, color.FgCyan)
	//Title of the quizz
	s.SetText(quizz.Title, 2, 2, color.FgHiRed, color.BgBlack)

	// Players Board
	s.DrawRect(56, 0, s.Columns-1, s.Rows-1, color.FgHiYellow)

}

func (s *PlayerScreen) DrawRect(topLeftX, topLeftY, bottomRightX, bottomRightY int, fg color.Attribute) {
	if topLeftX > s.Columns-1 || topLeftX < 0 {
		return
	}
	if bottomRightX > s.Columns-1 || bottomRightX < 0 {
		return
	}
	if topLeftY > s.Rows-1 || topLeftY < 0 {
		return
	}
	if bottomRightY > s.Rows-1 || bottomRightY < 0 {
		return
	}
	clr := color.New(fg)
	horizontalString := clr.Sprintf("%s", horizontalWall)
	verticalString := clr.Sprintf("%s", verticalWall)
	topLeftString := clr.Sprintf("%s", topLeft)
	topRightString := clr.Sprintf("%s", topRight)
	bottomLeftString := clr.Sprintf("%s", bottomLeft)
	bottomRightString := clr.Sprintf("%s", bottomRight)

	// Horizontal
	for j := topLeftX; j < bottomRightX; j++ {
		s.Tiles[topLeftY][j] = horizontalString
		s.Tiles[bottomRightY][j] = horizontalString
	}

	// Vertical
	for i := topLeftY; i < bottomRightY; i++ {
		s.Tiles[i][topLeftX] = verticalString
		s.Tiles[i][bottomRightX] = verticalString
	}

	// Corners
	s.Tiles[topLeftY][topLeftX] = topLeftString
	s.Tiles[topLeftY][bottomRightX] = topRightString
	s.Tiles[bottomRightY][bottomRightX] = bottomRightString
	s.Tiles[bottomRightY][topLeftX] = bottomLeftString

}

func (s *PlayerScreen) SetText(text string, i, j int, fg, bg color.Attribute) {

	if i > (s.Rows - 1) {
		return
	}

	if i+len(text) > s.Columns-1 {
		text = text[0 : len(text)-s.Columns]
	}

	clr := color.New(fg, bg)

	for iter := 0; iter < len(text); iter++ {
		s.Tiles[i][j+iter] = clr.Sprintf("%s", string(text[iter]))
	}
}

// NewPlayerSession creates a new session
func NewPlayerSession(channel ssh.Channel, name string, rows, columns int) *PlayerSession {

	return &PlayerSession{
		channel: channel,
		name:    name,
		answers: make(map[AnswerKey]string),
		screen:  NewPlayerScreen(rows, columns),
	}
}

func (s *PlayerSession) Update(quizz *Quizz) {

	var buffer bytes.Buffer
	s.screen.Compute(quizz)

	buffer.WriteString(clearScreen)
	buffer.WriteString(s.screen.Render())
	buffer.WriteString(hideCursor)

	io.Copy(s.channel, &buffer)
}

// AnswerKey is the key for the answer
type AnswerKey string

// Question represents a question
type Question struct {
	Timeout     time.Duration
	Content     string
	Answers     map[AnswerKey]string
	ValidAnswer AnswerKey
}

// Quizz is a set of questions
type Quizz struct {
	Title         string
	Difficulty    int
	Questions     []Question
	questionIndex int
}

// A QuizzServer handling quizzes
type QuizzServer struct {
	mux          sync.Mutex
	currentQuizz *Quizz
	Sessions     map[string]*PlayerSession
}

func (q *QuizzServer) AddSession(username string, session *PlayerSession) {
	q.mux.Lock()
	defer q.mux.Unlock()

	q.Sessions[username] = session
}

// NewQuizzServer returns a new game server
func NewQuizzServer() *QuizzServer {
	return &QuizzServer{
		Sessions: make(map[string]*PlayerSession),
	}
}

func (q *QuizzServer) Run() error {

	if q.currentQuizz == nil {
		panic("No start quizz has been set")
	}

	go func() {
		c := time.Tick(time.Second / 60)
		var lastUpdate time.Time

		for now := range c {
			q.Update(float64(now.Sub(lastUpdate)) / float64(time.Millisecond))
			lastUpdate = now
		}

	}()

	return nil
}

func (q *QuizzServer) Update(delta float64) {

	for _, session := range q.Sessions {
		session.Update(q.currentQuizz)
	}
}

func (q *QuizzServer) SetQuizz(quizz *Quizz) {
	q.mux.Lock()
	defer q.mux.Unlock()

	//TODO: Clear previous quizz

	// Assign new elements
	q.currentQuizz = quizz
}

func (q *QuizzServer) HandleUserConnection(channel ssh.Channel, username string, rows, columns int) {
	q.mux.Lock()
	defer q.mux.Unlock()

	session := NewPlayerSession(channel, username, rows, columns)
	q.Sessions[username] = session

}

func (q *QuizzServer) UpdateWindowSize(username string, rows, columns int) {
	session, ok := q.Sessions[username]
	if ok {
		fmt.Printf("Update windows for %s \n", username)
		session.UpdateWindows(rows, columns)
	}
}
