package main

import (
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

type ftpClient struct {
	conn net.Conn
	text *textproto.Conn
}

type ftpStatusError struct {
	op   string
	code int
	msg  string
}

func (e *ftpStatusError) Error() string {
	return fmt.Sprintf("ftp %s failed: %s", e.op, e.msg)
}

func newFTPClient(addr string) (*ftpClient, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}

	c := &ftpClient{conn: conn, text: textproto.NewConn(conn)}
	code, msg, err := c.read()
	if err != nil {
		conn.Close()
		return nil, err
	}
	if code >= 400 {
		conn.Close()
		return nil, &ftpStatusError{op: "connect", code: code, msg: msg}
	}
	return c, nil
}

func (c *ftpClient) Login(user, pass string) error {
	code, _, err := c.cmd("USER %s", user)
	if err != nil {
		return err
	}
	if code == 230 {
		return nil
	}
	code, msg, err := c.cmd("PASS %s", pass)
	if err != nil {
		return err
	}
	if code >= 400 {
		return &ftpStatusError{op: "login", code: code, msg: msg}
	}
	return nil
}

func (c *ftpClient) Size(file string) (int64, error) {
	code, msg, err := c.cmd("SIZE %s", file)
	if err != nil {
		return 0, err
	}
	if code >= 400 {
		return 0, &ftpStatusError{op: "size", code: code, msg: msg}
	}

	fields := strings.Fields(msg)
	n, err := strconv.ParseInt(fields[len(fields)-1], 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (c *ftpClient) List(path string) (io.ReadCloser, error) {
	dataConn, err := c.pasv()
	if err != nil {
		return nil, err
	}

	code, msg, err := c.cmd("LIST %s", path)
	if err != nil {
		dataConn.Close()
		return nil, err
	}
	if code >= 400 {
		dataConn.Close()
		return nil, &ftpStatusError{op: "list", code: code, msg: msg}
	}

	return &ftpDataConn{ReadCloser: dataConn, client: c}, nil
}

func (c *ftpClient) Retrieve(file string) (io.ReadCloser, error) {
	dataConn, err := c.pasv()
	if err != nil {
		return nil, err
	}

	code, msg, err := c.cmd("RETR %s", file)
	if err != nil {
		dataConn.Close()
		return nil, err
	}
	if code >= 400 {
		dataConn.Close()
		return nil, &ftpStatusError{op: "retr", code: code, msg: msg}
	}

	return &ftpDataConn{ReadCloser: dataConn, client: c}, nil
}

func (c *ftpClient) Close() error {
	c.cmd("QUIT")
	return c.text.Close()
}

func (c *ftpClient) pasv() (net.Conn, error) {
	_, msg, err := c.cmd("PASV")
	if err != nil {
		return nil, err
	}

	start := strings.Index(msg, "(")
	end := strings.Index(msg, ")")
	if start == -1 || end == -1 {
		return nil, fmt.Errorf("bad PASV response")
	}

	parts := strings.Split(msg[start+1:end], ",")
	if len(parts) != 6 {
		return nil, fmt.Errorf("bad PASV response")
	}

	host := strings.Join(parts[:4], ".")
	p1, _ := strconv.Atoi(parts[4])
	p2, _ := strconv.Atoi(parts[5])
	port := p1*256 + p2

	return net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 10*time.Second)
}

func (c *ftpClient) cmd(format string, args ...any) (int, string, error) {
	if _, err := c.text.Cmd(format, args...); err != nil {
		return 0, "", err
	}
	return c.read()
}

func (c *ftpClient) read() (int, string, error) {
	line, err := c.text.ReadLine()
	if err != nil {
		return 0, "", err
	}
	if len(line) < 3 {
		return 0, "", fmt.Errorf("bad ftp response")
	}

	code, err := strconv.Atoi(line[:3])
	if err != nil {
		return 0, "", err
	}
	if len(line) == 3 || line[3] != '-' {
		return code, line, nil
	}

	// FTP multiline replies start with "<code>-" and end with "<code> ".
	for {
		line, err = c.text.ReadLine()
		if err != nil {
			return 0, "", err
		}
		if len(line) >= 4 && line[:3] == fmt.Sprintf("%03d", code) && line[3] == ' ' {
			return code, line, nil
		}
	}
}

type ftpDataConn struct {
	io.ReadCloser
	client *ftpClient
}

func (d *ftpDataConn) Close() error {
	err := d.ReadCloser.Close()
	_, _, readErr := d.client.read()
	if err != nil {
		return err
	}
	return readErr
}
