package auth

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// PasswordPolicy 密码强度策略配置.
type PasswordPolicy struct {
	MinLength           int
	RequireUppercase    bool
	RequireLowercase    bool
	RequireDigit        bool
	RequireSpecial      bool
	SpecialChars        string
	DisallowedPasswords []string
}

// DefaultPasswordPolicy 返回默认密码策略：最少 8 位，需大小写和数字.
// 返回值：默认密码策略.
func DefaultPasswordPolicy() *PasswordPolicy {
	return &PasswordPolicy{
		MinLength:        8,
		RequireUppercase: true,
		RequireLowercase: true,
		RequireDigit:     true,
		SpecialChars:     "!@#$%^&*()_+-=[]{}|;:',.<>?/~`",
	}
}

// Validate 校验密码是否符合策略，返回未满足的规则列表.
// 参数：password - 待校验密码.
// 返回值：errs - 未满足规则列表，nil/空表示通过.
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

// Hasher 密码哈希接口，实现此接口可替代 bcrypt.
type Hasher interface {
	Hash(password string) (string, error)
	Verify(password, hashed string) bool
}

// BcryptHasher 默认 bcrypt 密码哈希器.
type BcryptHasher struct {
	Cost int // 0 使用 bcrypt.DefaultCost
}

// Hash 对密码进行 bcrypt 哈希.
// 参数：password - 原始密码.
// 返回值：hashed - 哈希结果, err - 哈希失败时的错误.
func (h BcryptHasher) Hash(password string) (string, error) {
	cost := h.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return string(b), err
}

// Verify 校验密码是否与哈希匹配.
// 参数：password - 原始密码, hashed - 已存储的哈希.
// 返回值：是否匹配.
func (h BcryptHasher) Verify(password, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)) == nil
}
