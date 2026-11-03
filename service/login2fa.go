package service

// obhod: подтверждение входа в панель кодом из Telegram. После верного пароля панель отправляет боту
// (адрес уведомлений) событие login_code, бот присылает код главному администратору. Код живёт 5 минут,
// даётся 5 попыток. Если бот недоступен, войти нельзя — выключить: s-ui setting -login2fa false (по SSH).

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"sync"
	"time"
)

type pendingCode struct {
	code  string
	exp   time.Time
	tries int
}

var (
	codesMu    sync.Mutex
	loginCodes = map[string]*pendingCode{}
)

const codeTTL = 5 * time.Minute

// StartLoginCode создаёт код и отправляет его боту. Ошибка — код не доставлен.
func StartLoginCode(user, ip string) error {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	if err := SendPanelEventSync("login_code", map[string]interface{}{"user": user, "ip": ip, "code": code}); err != nil {
		return fmt.Errorf("не удалось отправить код входа через бота: %v. Выключить проверку: s-ui setting -login2fa false", err)
	}
	codesMu.Lock()
	loginCodes[user] = &pendingCode{code: code, exp: time.Now().Add(codeTTL)}
	codesMu.Unlock()
	return nil
}

// CheckLoginCode — верен ли код (код одноразовый; после 5 неверных попыток нужно запросить новый).
func CheckLoginCode(user, code string) bool {
	codesMu.Lock()
	defer codesMu.Unlock()
	p := loginCodes[user]
	if p == nil || time.Now().After(p.exp) || p.tries >= 5 {
		delete(loginCodes, user)
		return false
	}
	p.tries++
	if subtle.ConstantTimeCompare([]byte(p.code), []byte(code)) != 1 {
		return false
	}
	delete(loginCodes, user)
	return true
}
