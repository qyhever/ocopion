package auth

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}
type TokenResponse struct {
	UserID       int64  `json:"-"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}
