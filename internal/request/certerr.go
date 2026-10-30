package request

import (
	"crypto/x509"
	"errors"
)

// isCertError matches the several concrete types the tls package uses for
// "I don't trust this certificate". errors.As needs a pointer to each type.
func isCertError(err error, out *interface{ Error() string }) bool {
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	var host x509.HostnameError
	switch {
	case errors.As(err, &unknown):
		*out = unknown
	case errors.As(err, &invalid):
		*out = invalid
	case errors.As(err, &host):
		*out = host
	default:
		return false
	}
	return true
}
