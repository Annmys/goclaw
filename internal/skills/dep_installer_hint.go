package skills

import "strings"

func pipBuildFailHint(pkg string, stderr string) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "pg_config executable not found"):
		return "psycopg2 缺少 pg_config，可改用 pip:psycopg2-binary 或安装 PostgreSQL 开发头文件。"
	case strings.Contains(lower, "fatal error: openssl") || strings.Contains(lower, "libssl"):
		return "缺少 OpenSSL 开发库，建议安装系统依赖后重试。"
	case strings.Contains(lower, "mysql_config") || strings.Contains(lower, "mariadb_config"):
		return "缺少 MySQL/MariaDB 开发工具，可改用 mysqlclient 对应系统依赖。"
	case strings.Contains(lower, "rust compiler"):
		return "该包需要 Rust 编译器，建议换二进制 wheel 或补装 Rust。"
	case strings.Contains(lower, "failed building wheel"):
		return "该 pip 包构建 wheel 失败，通常是缺少系统头文件或编译工具链。"
	default:
		_ = pkg
		return ""
	}
}
