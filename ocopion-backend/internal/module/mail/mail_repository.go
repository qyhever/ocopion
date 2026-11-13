package mail

type Repository interface {
	Send(to, subject, body string) error
}
