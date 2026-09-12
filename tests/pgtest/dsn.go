// Package pgtest 仅供本地 PostgreSQL 集成测试使用。
package pgtest

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// ValidateDSN 拒绝可能指向业务数据库、远端服务或隐式凭据来源的测试连接。
// 错误不包含输入，防止 DSN 的密码进入测试输出和阶段证据。
func ValidateDSN(raw string) (*url.URL, error) {
	invalid := errors.New("测试 DSN 必须显式使用 loopback IP、端口、urbino_test_admin 用户、非空密码、urbino_test 数据库和 sslmode=disable")
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Opaque != "" || u.Fragment != "" {
		return nil, invalid
	}
	if u.User == nil || u.User.Username() != "urbino_test_admin" {
		return nil, invalid
	}
	password, present := u.User.Password()
	if !present || password == "" {
		return nil, invalid
	}
	host, err := netip.ParseAddr(u.Hostname())
	if err != nil || !host.IsLoopback() || host.Zone() != "" {
		return nil, invalid
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || u.Path != "/urbino_test" || u.RawPath != "" {
		return nil, invalid
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query) != 1 || len(query["sslmode"]) != 1 || query.Get("sslmode") != "disable" {
		return nil, invalid
	}
	return u, nil
}

// ValidateEnvironment 防止驱动隐式继承 PGSERVICE、PGPASSFILE 或 PGOPTIONS 等配置。
func ValidateEnvironment(environ []string) error {
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(strings.ToUpper(name), "PG") {
			return errors.New("PostgreSQL 集成测试拒绝继承 PG* 环境变量；仅使用 URBINO_TEST_DATABASE_URL")
		}
	}
	return nil
}
