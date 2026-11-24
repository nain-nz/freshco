package database

import (
	"database/sql"
	"embed"
	"net"
	"time"

	"github.com/go-sql-driver/mysql"

	"smartfood/internal/config"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func dsn(cfg config.Config, withDB bool) string {
	c := mysql.NewConfig()
	c.User = cfg.DBUser
	c.Passwd = cfg.DBPassword
	c.Net = "tcp"
	c.Addr = net.JoinHostPort(cfg.DBHost, cfg.DBPort)
	if withDB {
		c.DBName = cfg.DBName
	}
	c.ParseTime = true
	c.Loc = time.Local
	c.MultiStatements = true
	return c.FormatDSN()
}

// EnsureDatabase membuat database kalau belum ada.
func EnsureDatabase(cfg config.Config) error {
	db, err := sql.Open("mysql", dsn(cfg, false))
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS `" + cfg.DBName +
		"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	return err
}

// Open membuka koneksi ke database dan memastikan koneksinya hidup.
func Open(cfg config.Config) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn(cfg, true))
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
