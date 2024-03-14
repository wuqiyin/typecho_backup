package main

import (
	"encoding/json"
	"fmt"
	"github.com/TruthHun/CloudStore"
	"github.com/jordan-wright/email"
	"github.com/robfig/cron/v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"net/smtp"
	"os"
	"os/exec"
	"time"
)

type ConfApp struct {
	ConfigQiNiu    ConfigQiNiu `json:"configQiNiu"`
	ToZipDir       string      `json:"toZipDir"`
	MysqlCfg       MysqlCfg    `json:"mysqlCfg"`
	CronSpec       string      `json:"cronSpec"`
	ReviewCronSpec string      `json:"reviewCronSpec"`
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
		sendMail("宕机通知", fmt.Sprintf("%v", err))
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
	sendMail("备份成功", "success")
}

type ReviewHandler struct {
}

type BlogRelationships struct {
	Cid int `json:"cid"`
	Mid int `json:"mid"`
}

type BlogReview struct {
	Id    int `json:"id" gorm:"primary_key"`
	Cid   int `json:"cid"`
	Count int `json:"count"`
}

type BlogContents struct {
	Cid   int    `json:"cid" gorm:"primary_key"`
	Title string `json:"title"`
}

func (h ReviewHandler) Run() {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		appCfg.MysqlCfg.User, appCfg.MysqlCfg.Password, appCfg.MysqlCfg.Host, appCfg.MysqlCfg.Database)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err) // 异常处理
	}

	var blogRelationships []BlogRelationships
	db.Where("mid = ?", 41).Find(&blogRelationships)
	var cids []int
	for _, ele := range blogRelationships {
		cids = append(cids, ele.Cid)
	}

	var blogReviews []BlogReview
	db.Where("cid NOT IN (?)", cids).Find(&blogReviews)
	for _, ele := range blogReviews {
		db.Delete(ele)
	}

	var insertData []BlogReview
	db.Find(&blogReviews)
	cidsMap := make(map[int]bool)
	minCount := 1000 // 不可能大于这个数量
	for _, ele := range blogReviews {
		cidsMap[ele.Cid] = true
		if ele.Count < minCount {
			minCount = ele.Count // 默认取最小值
		}
	}
	for _, ele := range cids {
		if _, ok := cidsMap[ele]; !ok {
			insertData = append(insertData, BlogReview{
				Cid:   ele,
				Count: minCount,
			})
		}
	}
	if len(insertData) > 0 {
		db.Create(insertData)
	}

	var reviewBlog BlogReview
	db.Order("Count ASC,cid ASC").First(&reviewBlog)
	if reviewBlog.Id > 0 {
		var blogContent BlogContents
		db.Where("cid = ?", reviewBlog.Cid).First(&blogContent)
		sendMail(
			fmt.Sprintf("博客复习：%s", blogContent.Title),
			fmt.Sprintf("链接地址：https://www.xiaoqiuyinboke.cn/archives/%d.html", blogContent.Cid),
		)

		reviewBlog.Count += 1
		db.Where("cid = ?", reviewBlog.Cid).Updates(reviewBlog)
	}
}

func sendMail(title, content string) {
	// 实例化邮件对象
	em := email.NewEmail()
	// 发送方邮箱
	em.From = "silverwq@qq.com"
	// 接收方邮箱
	em.To = []string{"silverwq@qq.com"}
	// 邮件标题
	em.Subject = title
	// 邮件内容
	em.Text = []byte(content)
	// 发送邮件 xxxxxxxxx 为刚才生成的授权码
	err := em.Send(
		"smtp.qq.com:587",
		smtp.PlainAuth("", "silverwq@qq.com", "", "smtp.qq.com"),
	)

	if err != nil {
		log.Fatalf("em.Send is failes, err: %v", err)
		return
	}
	log.Println("send successfully...")
}

func main() {
	defer func() {
		if err := recover(); err != nil {
			sendMail("宕机通知", fmt.Sprintf("%v", err))
		}
	}()

	err := loadConfig()
	if err != nil {
		log.Fatalln(err)
		return
	}

	// 备份
	handler := BackupHandler{}
	timer := cron.New(cron.WithSeconds())
	_, err = timer.AddJob(appCfg.CronSpec, handler)
	if err != nil {
		log.Fatalln(err)
		return
	}
	timer.Start()
	defer timer.Stop()

	// 复习
	reviewHandler := ReviewHandler{}
	reviewTimer := cron.New(cron.WithSeconds())
	_, err = reviewTimer.AddJob(appCfg.ReviewCronSpec, reviewHandler)
	if err != nil {
		log.Fatalln(err)
		return
	}
	reviewTimer.Start()
	defer reviewTimer.Stop()

	for {
		time.Sleep(time.Second)
	}

	return
}
