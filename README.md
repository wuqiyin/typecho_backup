# typecho_backup

用于在服务器上备份代码图片等数据到七牛云上。


首先在当前目录下创建一个配置文件cfg.json，内容如下，根据自己实际情况填写

```json
{
    "toZipDir":"/mydatata/typecho", // 博客的项目目录
    "cronSpec":"0 30 22 * * *", // 每天晚上十点半定时执行
    "mysqlCfg":{
        "dumpFilePath": "/usr/local/mysql/bin/mysqldump",// MySQL导出脚本目录
        "host": "127.0.0.1",
        "user": "root",
        "password": "123456",
        "port": "3306",
        "database": "a8oz00qmy"
    },
    "configQiNiu": {
        "accessKey": "",
        "secretKey": "",
        "privateBucket": "",
        "privateBucketDomain": "",
        "expire": 0
    }
}
```

然后执行

```bash
nohup ./typecho_backup & > backup.log
```

