package database

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	protocolVersion = 196608
	authOK          = 0
	authCleartext   = 3
	authMD5         = 5
	authSASL        = 10
	authSASLCont    = 11
	authSASLFinal   = 12
)

type Client struct {
	cfg connConfig
}

type connConfig struct {
	host     string
	port     string
	user     string
	password string
	database string
}

type pgConn struct {
	conn net.Conn
	cfg  connConfig
}

func New(databaseURL string) (*Client, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("database URL is required")
	}

	cfg, err := parseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	return &Client{cfg: cfg}, nil
}

func (c *Client) Close() error {
	return nil
}

func (c *Client) Query(ctx context.Context, sql string, args ...any) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	conn, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.close()

	expandedSQL, err := expandQuery(sql, args...)
	if err != nil {
		return nil, err
	}

	result, err := conn.query(ctx, expandedSQL)
	if err != nil {
		return nil, err
	}
	if result.rows != 1 || result.cols != 1 {
		return nil, fmt.Errorf("expected single-row single-column result, got %d rows and %d columns", result.rows, result.cols)
	}
	return result.value, nil
}

func (c *Client) Exec(ctx context.Context, sql string, args ...any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	conn, err := c.connect(ctx)
	if err != nil {
		return err
	}
	defer conn.close()

	expandedSQL, err := expandQuery(sql, args...)
	if err != nil {
		return err
	}

	return conn.exec(ctx, expandedSQL)
}

func (c *Client) Ping(ctx context.Context) error {
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.Exec(pingCtx, "SELECT 1;")
}

func Quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func expandQuery(sql string, args ...any) (string, error) {
	if len(args) == 0 {
		return sql, nil
	}

	var builder strings.Builder
	for i := 0; i < len(sql); i++ {
		if sql[i] != '$' {
			builder.WriteByte(sql[i])
			continue
		}

		j := i + 1
		for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
			j++
		}
		if j == i+1 {
			builder.WriteByte(sql[i])
			continue
		}

		idx, err := strconv.Atoi(sql[i+1 : j])
		if err != nil || idx < 1 || idx > len(args) {
			return "", fmt.Errorf("invalid SQL parameter reference %s", sql[i:j])
		}

		formatted, err := formatArg(args[idx-1])
		if err != nil {
			return "", fmt.Errorf("format SQL parameter %d: %w", idx, err)
		}
		builder.WriteString(formatted)
		i = j - 1
	}

	return builder.String(), nil
}

func formatArg(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "NULL", nil
	case string:
		return Quote(v), nil
	case []byte:
		return Quote(string(v)), nil
	case time.Time:
		return Quote(v.UTC().Format(time.RFC3339Nano)), nil
	case bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case int:
		return strconv.Itoa(v), nil
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case float32, float64:
		return fmt.Sprintf("%v", v), nil
	default:
		return "", fmt.Errorf("unsupported parameter type %T", value)
	}
}

func ReadSQLFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read SQL file %q: %w", path, err)
	}
	return string(content), nil
}

func parseConfig(databaseURL string) (connConfig, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return connConfig{}, fmt.Errorf("parse database URL: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return connConfig{}, fmt.Errorf("unsupported database scheme %q", u.Scheme)
	}
	sslmode := strings.ToLower(u.Query().Get("sslmode"))
	if sslmode != "" && sslmode != "disable" {
		return connConfig{}, fmt.Errorf("unsupported sslmode %q: only disable is supported", sslmode)
	}
	user := ""
	password := ""
	if u.User != nil {
		user = u.User.Username()
		password, _ = u.User.Password()
	}
	if user == "" {
		return connConfig{}, errors.New("database user is required")
	}
	host := u.Hostname()
	if host == "" {
		host = "127.0.0.1"
	}
	port := u.Port()
	if port == "" {
		port = "5432"
	}
	database := strings.TrimPrefix(u.Path, "/")
	if database == "" {
		database = user
	}
	return connConfig{host: host, port: port, user: user, password: password, database: database}, nil
}

func (c *Client) connect(ctx context.Context) (*pgConn, error) {
	dialer := &net.Dialer{}
	netConn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(c.cfg.host, c.cfg.port))
	if err != nil {
		return nil, fmt.Errorf("postgres connect failed: %w", err)
	}
	conn := &pgConn{conn: netConn, cfg: c.cfg}
	if err := conn.startup(ctx); err != nil {
		conn.close()
		return nil, err
	}
	return conn, nil
}

func (c *pgConn) close() {
	if c != nil && c.conn != nil {
		_ = c.conn.Close()
	}
}

type queryResult struct {
	rows  int
	cols  int
	value []byte
}

func (c *pgConn) startup(ctx context.Context) error {
	body := bytes.Buffer{}
	writeInt32(&body, protocolVersion)
	writeCString(&body, "user")
	writeCString(&body, c.cfg.user)
	writeCString(&body, "database")
	writeCString(&body, c.cfg.database)
	writeCString(&body, "client_encoding")
	writeCString(&body, "UTF8")
	body.WriteByte(0)

	if err := c.writePacket(ctx, 0, body.Bytes()); err != nil {
		return fmt.Errorf("postgres startup failed: %w", err)
	}

	var (
		scram *scramState
	)
	for {
		msgType, payload, err := c.readMessage(ctx)
		if err != nil {
			return fmt.Errorf("postgres startup failed: %w", err)
		}
		switch msgType {
		case 'R':
			if len(payload) < 4 {
				return errors.New("postgres auth failed: malformed auth message")
			}
			code := int(binary.BigEndian.Uint32(payload[:4]))
			switch code {
			case authOK:
			case authCleartext:
				if err := c.sendPassword(ctx, c.cfg.password); err != nil {
					return err
				}
			case authMD5:
				if len(payload) < 8 {
					return errors.New("postgres auth failed: malformed md5 challenge")
				}
				if err := c.sendPassword(ctx, md5Password(c.cfg.user, c.cfg.password, payload[4:8])); err != nil {
					return err
				}
			case authSASL:
				state, initial, err := newSCRAM(c.cfg.user, c.cfg.password)
				if err != nil {
					return err
				}
				scram = state
				if err := c.sendSASLInitial(ctx, "SCRAM-SHA-256", initial); err != nil {
					return err
				}
			case authSASLCont:
				if scram == nil {
					return errors.New("postgres auth failed: unexpected SASL continue")
				}
				response, err := scram.continueAuth(string(payload[4:]))
				if err != nil {
					return err
				}
				if err := c.sendPassword(ctx, response); err != nil {
					return err
				}
			case authSASLFinal:
				if scram == nil {
					return errors.New("postgres auth failed: unexpected SASL final")
				}
				if err := scram.finish(string(payload[4:])); err != nil {
					return err
				}
			default:
				return fmt.Errorf("postgres auth failed: unsupported auth method %d", code)
			}
		case 'S', 'K', 'N':
			continue
		case 'Z':
			return nil
		case 'E':
			return parseError(payload)
		default:
			return fmt.Errorf("postgres startup failed: unexpected message %q", msgType)
		}
	}
}

func (c *pgConn) query(ctx context.Context, sql string) (queryResult, error) {
	if err := c.sendQuery(ctx, sql); err != nil {
		return queryResult{}, err
	}

	result := queryResult{}
	var rowSeen bool
	for {
		msgType, payload, err := c.readMessage(ctx)
		if err != nil {
			return queryResult{}, fmt.Errorf("postgres query failed: %w", err)
		}
		switch msgType {
		case 'T':
			result.cols = int(readInt16(payload[:2]))
		case 'D':
			colCount, values, err := parseDataRow(payload)
			if err != nil {
				return queryResult{}, fmt.Errorf("postgres query failed: %w", err)
			}
			if result.cols == 0 {
				result.cols = colCount
			}
			result.rows++
			if rowSeen {
				continue
			}
			rowSeen = true
			if colCount == 1 {
				result.value = values[0]
			}
		case 'C', 'I', 'N':
			continue
		case 'Z':
			return result, nil
		case 'E':
			return queryResult{}, parseError(payload)
		default:
			return queryResult{}, fmt.Errorf("postgres query failed: unexpected message %q", msgType)
		}
	}
}

func (c *pgConn) exec(ctx context.Context, sql string) error {
	if err := c.sendQuery(ctx, sql); err != nil {
		return err
	}
	for {
		msgType, payload, err := c.readMessage(ctx)
		if err != nil {
			return fmt.Errorf("postgres exec failed: %w", err)
		}
		switch msgType {
		case 'T', 'D', 'C', 'I', 'N':
			continue
		case 'Z':
			return nil
		case 'E':
			return parseError(payload)
		default:
			return fmt.Errorf("postgres exec failed: unexpected message %q", msgType)
		}
	}
}

func (c *pgConn) sendQuery(ctx context.Context, sql string) error {
	payload := append([]byte(sql), 0)
	if err := c.writePacket(ctx, 'Q', payload); err != nil {
		return fmt.Errorf("postgres query failed: %w", err)
	}
	return nil
}

func (c *pgConn) sendPassword(ctx context.Context, password string) error {
	payload := append([]byte(password), 0)
	if err := c.writePacket(ctx, 'p', payload); err != nil {
		return fmt.Errorf("postgres auth failed: %w", err)
	}
	return nil
}

func (c *pgConn) sendSASLInitial(ctx context.Context, mechanism string, initial string) error {
	body := bytes.Buffer{}
	writeCString(&body, mechanism)
	writeInt32(&body, int32(len(initial)))
	body.WriteString(initial)
	if err := c.writePacket(ctx, 'p', body.Bytes()); err != nil {
		return fmt.Errorf("postgres auth failed: %w", err)
	}
	return nil
}

func (c *pgConn) writePacket(ctx context.Context, msgType byte, payload []byte) error {
	if err := c.applyDeadline(ctx); err != nil {
		return err
	}
	var packet bytes.Buffer
	if msgType != 0 {
		packet.WriteByte(msgType)
	}
	writeInt32(&packet, int32(len(payload)+4))
	packet.Write(payload)
	_, err := c.conn.Write(packet.Bytes())
	return err
}

func (c *pgConn) readMessage(ctx context.Context) (byte, []byte, error) {
	if err := c.applyDeadline(ctx); err != nil {
		return 0, nil, err
	}
	header := make([]byte, 5)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return 0, nil, err
	}
	length := int(binary.BigEndian.Uint32(header[1:5]))
	if length < 4 {
		return 0, nil, fmt.Errorf("invalid message length %d", length)
	}
	payload := make([]byte, length-4)
	if _, err := io.ReadFull(c.conn, payload); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}

func (c *pgConn) applyDeadline(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if deadline, ok := ctx.Deadline(); ok {
		return c.conn.SetDeadline(deadline)
	}
	return c.conn.SetDeadline(time.Time{})
}

func parseDataRow(payload []byte) (int, [][]byte, error) {
	if len(payload) < 2 {
		return 0, nil, errors.New("malformed data row")
	}
	colCount := int(readInt16(payload[:2]))
	values := make([][]byte, 0, colCount)
	pos := 2
	for i := 0; i < colCount; i++ {
		if pos+4 > len(payload) {
			return 0, nil, errors.New("malformed data row length")
		}
		length := int(int32(binary.BigEndian.Uint32(payload[pos : pos+4])))
		pos += 4
		if length == -1 {
			values = append(values, nil)
			continue
		}
		if pos+length > len(payload) {
			return 0, nil, errors.New("malformed data row value")
		}
		value := make([]byte, length)
		copy(value, payload[pos:pos+length])
		values = append(values, value)
		pos += length
	}
	return colCount, values, nil
}

func parseError(payload []byte) error {
	parts := make([]string, 0, 4)
	for len(payload) > 1 {
		fieldType := payload[0]
		payload = payload[1:]
		idx := bytes.IndexByte(payload, 0)
		if idx < 0 {
			break
		}
		value := string(payload[:idx])
		payload = payload[idx+1:]
		if fieldType == 'M' || fieldType == 'D' {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return errors.New("postgres error")
	}
	return errors.New(strings.Join(parts, ": "))
}

func md5Password(user string, password string, salt []byte) string {
	inner := md5.Sum([]byte(password + user))
	innerHex := fmt.Sprintf("%x", inner[:])
	outerInput := append([]byte(innerHex), salt...)
	outer := md5.Sum(outerInput)
	return "md5" + fmt.Sprintf("%x", outer[:])
}

func writeCString(buf *bytes.Buffer, value string) {
	buf.WriteString(value)
	buf.WriteByte(0)
}

func writeInt32(buf *bytes.Buffer, value int32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(value))
	buf.Write(b[:])
}

func readInt16(b []byte) int16 {
	return int16(binary.BigEndian.Uint16(b))
}

type scramState struct {
	password           string
	clientFirstBare    string
	serverFirstMessage string
	clientNonce        string
	serverSignatureB64 string
}

func newSCRAM(user string, password string) (*scramState, string, error) {
	nonceBytes := make([]byte, 18)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, "", fmt.Errorf("generate scram nonce: %w", err)
	}
	nonce := base64.StdEncoding.EncodeToString(nonceBytes)
	escapedUser := strings.NewReplacer("=", "=3D", ",", "=2C").Replace(user)
	clientFirstBare := "n=" + escapedUser + ",r=" + nonce
	return &scramState{password: password, clientFirstBare: clientFirstBare, clientNonce: nonce}, "n,," + clientFirstBare, nil
}

func (s *scramState) continueAuth(serverFirst string) (string, error) {
	attrs := parseSCRAMAttributes(serverFirst)
	nonce := attrs["r"]
	if !strings.HasPrefix(nonce, s.clientNonce) {
		return "", errors.New("postgres auth failed: invalid SCRAM nonce")
	}
	salt, err := base64.StdEncoding.DecodeString(attrs["s"])
	if err != nil {
		return "", fmt.Errorf("postgres auth failed: decode SCRAM salt: %w", err)
	}
	iterations, err := strconv.Atoi(attrs["i"])
	if err != nil {
		return "", fmt.Errorf("postgres auth failed: invalid SCRAM iterations: %w", err)
	}
	clientFinalWithoutProof := "c=biws,r=" + nonce
	authMessage := s.clientFirstBare + "," + serverFirst + "," + clientFinalWithoutProof
	saltedPassword := pbkdf2SHA256([]byte(s.password), salt, iterations, 32)
	clientKey := hmacSHA256(saltedPassword, []byte("Client Key"))
	storedKey := sha256.Sum256(clientKey)
	clientSignature := hmacSHA256(storedKey[:], []byte(authMessage))
	proof := xorBytes(clientKey, clientSignature)
	serverKey := hmacSHA256(saltedPassword, []byte("Server Key"))
	serverSignature := hmacSHA256(serverKey, []byte(authMessage))
	s.serverFirstMessage = serverFirst
	s.serverSignatureB64 = base64.StdEncoding.EncodeToString(serverSignature)
	return clientFinalWithoutProof + ",p=" + base64.StdEncoding.EncodeToString(proof), nil
}

func (s *scramState) finish(serverFinal string) error {
	attrs := parseSCRAMAttributes(serverFinal)
	if attrs["v"] == "" {
		if attrs["e"] != "" {
			return fmt.Errorf("postgres auth failed: %s", attrs["e"])
		}
		return errors.New("postgres auth failed: missing SCRAM verifier")
	}
	if attrs["v"] != s.serverSignatureB64 {
		return errors.New("postgres auth failed: SCRAM verifier mismatch")
	}
	return nil
}

func parseSCRAMAttributes(msg string) map[string]string {
	attrs := map[string]string{}
	for _, part := range strings.Split(msg, ",") {
		if len(part) < 3 || part[1] != '=' {
			continue
		}
		attrs[part[:1]] = part[2:]
	}
	return attrs
}

func hmacSHA256(key []byte, msg []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return mac.Sum(nil)
}

func pbkdf2SHA256(password []byte, salt []byte, iterations int, keyLen int) []byte {
	hLen := 32
	numBlocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, numBlocks*hLen)
	for block := 1; block <= numBlocks; block++ {
		u := hmacSHA256(password, append(append([]byte{}, salt...), byte(block>>24), byte(block>>16), byte(block>>8), byte(block)))
		t := append([]byte{}, u...)
		for i := 1; i < iterations; i++ {
			u = hmacSHA256(password, u)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func xorBytes(a []byte, b []byte) []byte {
	out := make([]byte, len(a))
	for i := range a {
		out[i] = a[i] ^ b[i]
	}
	return out
}
