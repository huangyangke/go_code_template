package auth

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// PasswordPolicy defines password strength requirements.
type PasswordPolicy struct {
	MinLength           int
	RequireUppercase    bool
	RequireLowercase    bool
	RequireDigit        bool
	RequireSpecial      bool
	SpecialChars        string
	DisallowedPasswords []string
}

// DefaultPasswordPolicy returns the default policy: min 8 chars, upper+lower+digit required.
func DefaultPasswordPolicy() *PasswordPolicy {
	return &PasswordPolicy{
		MinLength:        8,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		SpecialChars:     "!@#$%^&*()_+-=[]{}|;:',.<>?/~`",
	}
}

// Validate returns all unmet policy rules; nil/empty means pass.
func (p *PasswordPolicy) Validate(password string) []string {
	var errs []string
	if len(password) < p.MinLength {
		errs = append(errs, fmt.Sprintf("密码长度不足，至少需要 %d 个字符", p.MinLength))
	}
	if p.RequireUppercase {
		ok := false
		for _, c := range password {
			if unicode.IsUpper(c) {
				ok = true
				break
			}
		}
		if !ok {
			errs = append(errs, "密码必须包含至少一个大写字母")
		}
	}
	if p.RequireLowercase {
		ok := false
		for _, c := range password {
			if unicode.IsLower(c) {
				ok = true
				break
			}
		}
		if !ok {
			errs = append(errs, "密码必须包含至少一个小写字母")
		}
	}
	if p.RequireDigit {
		ok := false
		for _, c := range password {
			if unicode.IsDigit(c) {
				ok = true
				break
			}
		}
		if !ok {
			errs = append(errs, "密码必须包含至少一个数字")
		}
	}
	if p.RequireSpecial {
		ok := false
		for _, c := range password {
			if strings.ContainsRune(p.SpecialChars, c) {
				ok = true
				break
			}
		}
		if !ok {
			errs = append(errs, fmt.Sprintf("密码必须包含至少一个特殊字符（%s）", p.SpecialChars))
		}
	}
	lower := strings.ToLower(password)
	for _, bad := range p.DisallowedPasswords {
		if lower == strings.ToLower(bad) {
			errs = append(errs, "该密码为常见弱密码，不允许使用")
			break
		}
	}
	return errs
}

// Hasher is the password hashing interface; implement it to replace bcrypt.
type Hasher interface {
	Hash(password string) (string, error)
	Verify(password, hashed string) bool
}

// BcryptHasher is the default bcrypt-based password hasher.
type BcryptHasher struct {
	Cost int // 0 uses bcrypt.DefaultCost
}

func (h BcryptHasher) Hash(password string) (string, error) {
	cost := h.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return string(b), err
}

func (h BcryptHasher) Verify(password, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)) == nil
}
