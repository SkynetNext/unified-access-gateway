package xlog

import (
	"fmt"
	"log"
	"os"
)

var logger = log.New(os.Stdout, "[GATEWAY] ", log.LstdFlags)

func Infof(format string, v ...interface{}) {
	logger.Printf("[INFO] "+format, v...)
}

func Errorf(format string, v ...interface{}) {
	logger.Printf("[ERROR] "+format, v...)
}

func Warnf(format string, v ...interface{}) {
	logger.Printf("[WARN] "+format, v...)
}

func Debugf(format string, v ...interface{}) {
	fmt.Printf("[DEBUG] "+format+"\n", v...)
}

