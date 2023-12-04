package main

import (
	"encoding/json"
	"fmt"
	"github.com/TruthHun/CloudStore"
	"github.com/robfig/cron/v3"
	"log"
	"os"
	"os/exec"
	"time"
)

type ConfApp struct {
	ConfigQiNiu ConfigQiNiu `json:"configQiNiu"`
	ToZipDir    string      `json:"toZipDir"`
	MysqlCfg    MysqlCfg    `json:"mysqlCfg"`
	CronSpec    string      `json:"cronSpec"`
}

type MysqlCfg struct {
	DumpFilePath string `json:"dumpFilePath"`
	Host         string `json:"host"`
	User         string `json:"user"`
	Password     string `json:"password"`
	Port         string `json:"port"`
	Database     string `json:"database"`
}

type ConfigQiNiu struct {
	AccessKey           string `json:"accessKey"`
	SecretKey           string `json:"secretKey"`
	PrivateBucket       string `json:"privateBucket"`
	PrivateBucketDomain string `json:"privateBucketDomain"`
	Expire              int64  `json:"expire"`
}

var appCfg ConfApp

func loadConfig() (err error) {
	content, err := os.ReadFile("./cfg.json")
	if err != nil {
		return
	}

	err = json.Unmarshal(content, &appCfg)
	if err != nil {
		return
	}

	return
}

func backupDatabases() (err error) {
	cmd := exec.Command(
		appCfg.MysqlCfg.DumpFilePath,
		"-h",
		appCfg.MysqlCfg.Host,
		"-u",
		appCfg.MysqlCfg.User,
		fmt.Sprintf("-p%s", appCfg.MysqlCfg.Password),
		"-P",
		appCfg.MysqlCfg.Port,
		"-B",
		appCfg.MysqlCfg.Database,
	)

	err = cleanBackupFile()
	if err != nil {
		return
	}

	err = os.Mkdir("./backup", os.ModePerm)
	if err != nil {
		err = fmt.Errorf("创建./backup目录失败，Err%s", err.Error())
		return
	}

	stdout, err := os.OpenFile("./backup/databases_backup.sql", os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		err = fmt.Errorf("创建./backup/databases_backup.sql文件失败，Err%s", err.Error())
		return
	}

	defer stdout.Close()
	// 重定向标准输出到文件
	cmd.Stdout = stdout
	// 执行命令
	if err = cmd.Start(); err != nil {
		err = fmt.Errorf("准备执行备份数据库失败，Err%s", err.Error())
		return
	}
	if err = cmd.Wait(); err != nil {
		err = fmt.Errorf("执行mysqldump备份数据库失败，Err%s", err.Error())
		return
	}

	return
}

func backupProjectFile() (err error) {
	cmd := exec.Command("zip", "-r", "./backup/backup_project.zip", appCfg.ToZipDir)
	// // 不需要cmd.Run()
	_, err = cmd.Output()
	if err != nil {
		err = fmt.Errorf("执行 zip -r ./backup/backup_project.zip 命令失败，Err%s", err.Error())
		return
	}
	return
}

func uploadQiNiuYUn(filePath string) (err error) {
	cfg := appCfg.ConfigQiNiu

	if cfg.AccessKey == "" || cfg.SecretKey == "" || cfg.PrivateBucket == "" {
		err = fmt.Errorf("七牛云配置缺少")
		return
	}

	client, err := CloudStore.NewQINIU(cfg.AccessKey, cfg.SecretKey, cfg.PrivateBucket, cfg.PrivateBucketDomain)
	if err != nil {
		err = fmt.Errorf("初始化七牛云客户端失败，Err%s", err.Error())
		return
	}

	err = client.Upload(filePath, fmt.Sprintf("backup_%s.zip", time.Now().Format("2006_01_02_15_04_05")))
	if err != nil {
		err = fmt.Errorf("七牛云上传失败，Err%s", err.Error())
		return
	}
	return
}

func cleanBackupFile() (err error) {
	_ = os.RemoveAll("./backup")
	_ = os.Remove("./backup.zip")
	return
}

type BackupHandler struct {
}

func (h BackupHandler) Run() {
	defer func() {
		err := cleanBackupFile()
		if err != nil {
			log.Fatalln(err)
		}
	}()

	err := backupDatabases()
	if err != nil {
		log.Fatalln(err)
		return
	}

	err = backupProjectFile()
	if err != nil {
		log.Fatalln(err)
		return
	}

	cmd := exec.Command("zip", "-r", "./backup.zip", "./backup")
	_, err = cmd.Output()
	if err != nil {
		err = fmt.Errorf("执行zip -r ./backup.zip ./backup 失败，Err%s", err.Error())
		return
	}

	err = uploadQiNiuYUn("./backup.zip")
	if err != nil {
		log.Fatalln(err)
		return
	}

	log.Println("备份成功")
}

func main() {
	err := loadConfig()
	if err != nil {
		log.Fatalln(err)
		return
	}

	handler := BackupHandler{}
	timer := cron.New(cron.WithSeconds())
	_, err = timer.AddJob(appCfg.CronSpec, handler)
	if err != nil {
		log.Fatalln(err)
		return
	}
	timer.Start()
	defer timer.Stop()

	for {
		time.Sleep(time.Second)
	}

	return
}
