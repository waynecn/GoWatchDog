package main

import (
    "path"
    "os"
    "fmt"
    "github.com/lestrrat/go-file-rotatelogs"
    "github.com/pkg/errors"
    "github.com/rifflock/lfshook"
    "os/exec"
    "io/ioutil"
    "encoding/json"
	"time"
	"github.com/sirupsen/logrus"
	"regexp"
	"runtime"
)

func ConfigLocalFileSystemLogger(logPath string, logFileName string, maxAge time.Duration, rotationTime time.Duration) {
	baseLogPath := path.Join(logPath, logFileName)
	fmt.Println("baseLogPath:", baseLogPath)
	writer, err := rotatelogs.New(baseLogPath + ".%Y%m%d%H%M",
		rotatelogs.WithLinkName(baseLogPath),
		rotatelogs.WithMaxAge(maxAge),
		rotatelogs.WithRotationTime(rotationTime),)
	if err != nil {
		fmt.Println("config local file system logger error, detail err:", err)
		logrus.Errorf("config local file system logger error. %+v", errors.WithStack(err))
	}
	
	lfHook := lfshook.NewHook(lfshook.WriterMap{
		logrus.DebugLevel : writer,
		logrus.InfoLevel : writer,
		logrus.WarnLevel : writer,
		logrus.ErrorLevel : writer,
		logrus.FatalLevel : writer,
		logrus.PanicLevel : writer,}, &logrus.TextFormatter{})
		
	logrus.AddHook(lfHook)
}

var g_configFileModifyTime time.Time
var jsonContent map[string]interface{}

func FileExists(fileName string) bool {
	fileinfo, err := os.Stat(fileName)
	if err != nil && os.IsNotExist(err) {
		fmt.Printf("file:%s doesn't exist.\n", fileName)
		logrus.Errorf("file:%s doesn't exist.", fileName)
		return false
	} else {
		g_configFileModifyTime = fileinfo.ModTime()
		return true
	}
}

func ReadConfigFile(confFile string) bool {
	if !FileExists(confFile) {
		logrus.Errorf("config file:%s doesn't exist.", confFile)
		return false
	}
	
	conf, err := os.Open(confFile)
	if err != nil {
		logrus.Errorf("failed to open config file:%s, error:%s", confFile, err.Error())
		return false
	}
	
	contents, err := ioutil.ReadAll(conf)
	if err != nil {
		logrus.Error("ioutil ReadAll failed.")
		return false
	}
	strContent := string(contents)
	re := regexp.MustCompile("\\n")
	strContent = re.ReplaceAllString(strContent, "")

	defer conf.Close()
	
	if jsonContent != nil {
		jsonContent = make(map[string]interface{})
	}
	err = json.Unmarshal([]byte(strContent), &jsonContent)
	if err != nil {
		logrus.Errorf("json格式化配置文件内容失败，错误描述：%s", err.Error())
		return false
	}
	
	return true
}

func main() {
	sysType := runtime.GOOS
	fmt.Println("sysType:", sysType)
	bWindows := false
	bLinux := false
	if sysType == "windows" {
		bWindows = true
	} else if sysType == "linux" {
		bLinux = true
	}
	absDir, err := os.Getwd()
	if err != nil {
		fmt.Println("获取程序工作目录失败，错误描述：" + err.Error())
		return
	}
	logDir := absDir + "/log"
	err = os.MkdirAll(logDir, 0777)
	if err != nil {
		fmt.Println("创建目录:", logDir, "失败")
		return
	}
	ConfigLocalFileSystemLogger("./log", "GoWatchDog.log", 30*24*time.Hour, 24*time.Hour)
	
	
	logrus.Infof("absDir:%s", absDir)
	confFile := path.Join(absDir, "./conf/watchList.conf")
	logrus.Infof("config file:%s", confFile)
	
	if !ReadConfigFile(confFile) {
		logrus.Error("读取配置文件失败")
		return
	}
	
	//检测配置文件中配置的程序是否在运行
	for {
		//检查配置文件是否有修改
		confinfo, err := os.Stat(confFile)
		if err != nil && os.IsNotExist(err) {
			logrus.Errorf("配置文件：%s不存在。", confFile)
		} else {
			modifyTime := confinfo.ModTime()
			if modifyTime != g_configFileModifyTime {
				logrus.Warnf("配置文件：%s被修改。", confFile)
				if !ReadConfigFile(confFile) {
					logrus.Error("读取配置文件失败")
					return
				}
				g_configFileModifyTime = confinfo.ModTime()
				logrus.Infof("修改时间:%s", g_configFileModifyTime)
			}
		}
		
		if bLinux {
			for key, val := range jsonContent {
				command := fmt.Sprintf("ps ax|grep -v 'grep'|grep '%s'|awk '{print $1}'", key)
				logrus.Infof("command:%s", command)
				out, err := exec.Command("/bin/sh", "-c", command).CombinedOutput()
				if err != nil {
					logrus.Errorf("执行命令：%s失败，错误描述：%s", command, err.Error())
					continue
				}
				
				for i := 0; i < len(out); i++ {
					if out[i] == '\n' {
						out = out[:i]
						break
					}
				}
				
				appname := fmt.Sprintf("%s/%s", val.(string), key)
				logrus.Infof("pid:[%s] | Application:[%s]", string(out), appname)
				
				if string(out) == "" {
					err = os.Chdir(val.(string))
					if err != nil {
						logrus.Errorf("切换到目录：%s失败", val.(string))
						continue
					}
					cmd := exec.Command(appname)
					err = cmd.Start()
					if err != nil {
						logrus.Errorf("唤起程序：%s失败，错误描述：%s", appname, err.Error())
						break
					}
				}
			}
		}
		if bWindows {
			//
			for key, val := range jsonContent {
				command := fmt.Sprintf("tasklist|findstr %s", key)
				logrus.Infof("command:%s", command)
				out, err := exec.Command("cmd", "/c", command).CombinedOutput()
				// if err != nil {
				// 	logrus.Errorf("执行命令：%s失败，错误描述：%s", command, err.Error())
				// 	continue
				// }
				if err != nil {
					logrus.Infof("执行命令：%s结果：%s", command, err.Error())
				}
				
				for i := 0; i < len(out); i++ {
					if out[i] == '\n' {
						out = out[:i]
						break
					}
				}
				
				appname := fmt.Sprintf("%s/%s", val.(string), key)
				logrus.Infof("pid:[%s] | Application:[%s]", string(out), appname)
				
				if string(out) == "" {
					err = os.Chdir(val.(string))
					if err != nil {
						logrus.Errorf("切换到目录：%s失败", val.(string))
						continue
					}
					cmd := exec.Command(appname)
					err = cmd.Start()
					if err != nil {
						logrus.Errorf("唤起程序：%s失败，错误描述：%s", appname, err.Error())
						break
					}
				}
			}
		}
		
		time.Sleep(2 * time.Second)
	}
	
	logrus.Error("GoWatchDog程序退出")
}
