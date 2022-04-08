package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"time"

	uuid "github.com/satori/go.uuid"
)

type Message struct {
	OwnerID string
	Content string
}

var (
	// 新用户到来，通过channel 进行登记
	enteringChannel = make(chan *User)
	// 用户离开，通过channel进行登记
	leavingChannel = make(chan *User)
	// 用于广播的消息
	messageChannel = make(chan Message, 8)
)

func main() {
	listener, err := net.Listen("tcp", ":2020")
	if err != nil {
		panic(err)
	}
	go broadcaster()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go handleConn(conn)
	}

}

type User struct {
	ID             string
	Addr           string
	EnterAt        time.Time
	MessageChannel chan Message
}

// GenUserID 生成用户id
func GenUserID() string {
	id := uuid.NewV4()
	return id.String()
}

// handleConn 处理连接
func handleConn(conn net.Conn) {
	defer conn.Close()
	// 1, 新用户进来构建，用户实例
	user := &User{
		ID:             GenUserID(),
		Addr:           conn.RemoteAddr().String(),
		EnterAt:        time.Now(),
		MessageChannel: make(chan Message, 8),
	}
	// 2, 当一个新的goroutine中，用来进行读操作，因此需要新开一个goroutine用于写操作
	go sendMessage(conn, user.MessageChannel)
	// 3, 给当前用户发送欢迎消息，给所有用户告知新用户到来
	user.MessageChannel <- Message{OwnerID: user.ID, Content: "Welcome," + user.ID}

	messageInfo := Message{OwnerID: user.ID, Content: "user:`" + user.ID + "` has enter"}
	messageChannel <- messageInfo
	// 4, 将该记录到全局的用户列表中，避免用锁
	enteringChannel <- user
	// 5, 循环读取用户输入
	input := bufio.NewScanner(conn)
	for input.Scan() {
		messageChannel <- Message{OwnerID: user.ID, Content: user.ID + ":" + input.Text()}
	}
	if err := input.Err(); err != nil {
		log.Println("读取错误：", err)
	}
	fmt.Println("用户离开。。。")
	// 6 用户离开
	leavingChannel <- user
	messageChannel <- Message{OwnerID: user.ID, Content: "user:`" + user.ID + "` has left"}

}

// sendMessage 发送消息
func sendMessage(conn net.Conn, ch <-chan Message) {
	for msg := range ch {
		fmt.Fprintln(conn, msg.Content)
	}
}

// broadcaster 用于记录聊天室用户，并进行消息广播：
// 1. 新用户进来；2. 用户普通消息；3. 用户离开
func broadcaster() {
	users := make(map[*User]struct{})
	for {
		select {
		case user := <-enteringChannel:
			// 新用户进入
			users[user] = struct{}{}
		case user := <-leavingChannel:
			// 用户离开
			delete(users, user)
			// 避免 goroutine 泄露
			close(user.MessageChannel)
		case msg := <-messageChannel:
			// 给所有在线用户发送消息
			for user := range users {
				if user.ID == msg.OwnerID {
					continue
				}
				user.MessageChannel <- msg
			}
		}
	}
}
