package mail

import "ocopion/internal/pkg/postal"

type PostalRepository struct{}

func NewPostalRepository() *PostalRepository { return &PostalRepository{} }

func (r *PostalRepository) Send(to, subject, body string) error {
	return postal.SendMail(to, subject, body)
}
