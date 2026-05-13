package auth

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var (
	secKey = []byte("-----BEGIN EC PARAMETERS-----\n" +
		"BgUrgQQACg==\n" +
		"-----END EC PARAMETERS-----\n" +
		"-----BEGIN EC PRIVATE KEY-----\n" +
		"MHQCAQEEIP5DyqGW1yUD5YZRSzsvjT5I9M1utN9aYi3uWJgKhsvPoAcGBSuBBAAK\n" +
		"oUQDQgAEOX+BUepYysBoGR3l9ZsnIXNBm4FYD6m76rGPvbJnUD11xm/SQrOALZYC\n" +
		"s0VrWcLTP60Z1xeLw+NP+D+rUK5IsA==\n" +
		"-----END EC PRIVATE KEY-----\n")

	keyID = "b38daf14d0e6f487949cefbccce99d8add909685"
)

func TestNewCorpAuthMaker(t *testing.T) {
	tests := map[string]struct {
		secKey    []byte
		wantKeyID string
		wantErr   error
	}{
		"positive": {
			secKey:    secKey,
			wantKeyID: keyID,
		},
		"negative": {
			secKey:  []byte("invalid"),
			wantErr: errors.New("failed to decode private key"),
		},
	}

	for name, tt := range tests {
		tt := tt
		t.Run(name, func(t *testing.T) {
			got, err := NewCorpAuthMaker(tt.secKey)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.NotNil(t, got.privateKey)
			assert.Equal(t, tt.wantKeyID, got.KeyID)
		})
	}
}

type CorpSuite struct {
	suite.Suite
	maker *CorpAuthMaker
}

func TestCorpSuite(t *testing.T) {
	suite.Run(t, new(CorpSuite))
}

func (s *CorpSuite) SetupTest() {
	m, err := NewCorpAuthMaker(secKey)
	s.Require().NoError(err)
	s.maker = m
}

func (s *CorpSuite) Test_sign() {
	a, ok := s.maker.NewPermissions(PermPI).(Corp)
	s.Require().True(ok)

	got, err := a.sign("1136239445", "p", "/personal/auth/request")
	s.Require().NoError(err)
	// Signature length is fixed at 96 base64 chars; the bytes themselves
	// vary because ECDSA uses a fresh random nonce per call.
	s.Assert().Len(got, 96)
}
