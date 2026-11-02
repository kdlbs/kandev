package webpush

import (
	webpushlib "github.com/SherClockHolmes/webpush-go"

	"github.com/kandev/kandev/internal/notifications/models"
)

func generateVAPIDKeys() (models.VAPIDKeys, error) {
	priv, pub, err := webpushlib.GenerateVAPIDKeys()
	if err != nil {
		return models.VAPIDKeys{}, err
	}
	return models.VAPIDKeys{PrivateKey: priv, PublicKey: pub}, nil
}
