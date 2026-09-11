package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/util"
)

// Claims JWT 载荷
type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     int    `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken 签发 JWT（有效期 7 天）
func GenerateToken(userID uint, username string, role int) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(config.Cfg.JWTExpireDay) * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "happy-cloud",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.Cfg.JWTSecret))
}

// ParseToken 解析并校验 JWT
func ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		return []byte(config.Cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}

// Auth 鉴权中间件：校验 Authorization: Bearer <token>
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			util.Fail(c, 401, "未登录或登录已过期")
			c.Abort()
			return
		}
		claims, err := ParseToken(strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			util.Fail(c, 401, "未登录或登录已过期")
			c.Abort()
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// Admin 管理员中间件：需在 Auth 之后使用
func Admin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if role, _ := c.Get("role"); role != 1 {
			util.Fail(c, 403, "无管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CORS 跨域中间件
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allow := ""
		for _, o := range config.Cfg.CORSOrigins {
			if o == "*" || o == origin {
				allow = o
				break
			}
		}
		// L12 修复：未匹配白名单的 Origin 不再按原样回显（原逻辑会把任意 Origin 放行并附带
		// Access-Control-Allow-Credentials: true），避免任意站点获得跨域权限；allow 为空时下方不设置任何 CORS 头
		if allow != "" {
			c.Header("Access-Control-Allow-Origin", allow)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Share-Password")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
