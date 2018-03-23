package pub

import (
	"fmt"
	"log"
	"os"
	"time"
)

const (
	ErrorLog = "error.log"
)

var gFile *os.File

//年月日日志
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
	log.Println(fmt.Sprint(v...))
	logger.Println(fmt.Sprint(v...))
}

//特定文件日志
func PrintFileLog(filename string, v ...interface{}) {
	file, err := os.OpenFile("./log/"+filename+".log", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0660)
	if err != nil {
		log.Println("log open filename:", filename, ", error:", err.Error())
		return
	}
	logger := log.New(file, "", log.LstdFlags)
	log.Println(fmt.Sprint(v...))
	logger.Println(fmt.Sprint(v...))
	file.Close()
}
