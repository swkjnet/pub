package pub

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"time"
)

var gFile *os.File

func init() {
	logFile, err := os.OpenFile("log/sysdebug.log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		log.Println("open sysdebug err:", err.Error())
		return
	}
	// 将进程标准出错重定向至文件，进程崩溃时运行时将向该文件记录协程调用栈信息
	syscall.Dup2(int(logFile.Fd()), int(os.Stderr.Fd()))
}

func PrintLog(v ...interface{}) {
	fileName := "./log/" + fmt.Sprintf("%d_%d_%d.log", time.Now().Year(), time.Now().Month(), time.Now().Day())
	if gFile == nil || gFile.Name() != fileName {
		if gFile != nil {
			gFile.Close()
		}
		var err error
		gFile, err = os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
		if err != nil {
			log.Println("log open error:", err.Error())
			return
		}
	}
	logger := log.New(gFile, "", log.LstdFlags)
	logger.Println(fmt.Sprint(v...))
}

func PrintFileLog(filename string, v ...interface{}) {
	file, err := os.OpenFile("./log/"+filename+".log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		log.Println("log open filename:", filename, ", error:", err.Error())
		return
	}
	logger := log.New(file, "", log.LstdFlags)
	logger.Println(fmt.Sprint(v...))
	file.Close()
}
