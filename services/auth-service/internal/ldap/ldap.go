package ldapauth

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"time"

	"esppd.local/shared/config"
	"github.com/go-ldap/ldap/v3"
)

type Client struct {
	cfg config.LDAP
}

func New(cfg config.LDAP) *Client {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "disabled"
	}
	cfg.Mode = mode
	return &Client{cfg: cfg}
}

func (c *Client) Enabled() bool {
	if c == nil {
		return false
	}
	return c.cfg.Mode != "disabled" && c.cfg.URL != "" && c.cfg.BaseDN != ""
}

// Authenticate validates username/password against LDAP using:
// 1) (optional) service bind
// 2) search user DN with LDAP_USER_FILTER (replace {username})
// 3) bind as user DN with supplied password
func (c *Client) Authenticate(ctx context.Context, username, password string) (bool, error) {
	if !c.Enabled() {
		return false, nil
	}
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return false, nil
	}

	dialer := &net.Dialer{Timeout: c.cfg.Timeout}
	conn, err := ldap.DialURL(c.cfg.URL, ldap.DialWithDialer(dialer))
	if err != nil {
		return false, err
	}
	defer conn.Close()

	if c.cfg.StartTLS {
		tlsCfg := &tls.Config{InsecureSkipVerify: c.cfg.InsecureSkipVerify} // #nosec G402 (configurable for dev)
		if err := conn.StartTLS(tlsCfg); err != nil {
			return false, err
		}
	}

	// Deadline best-effort (ldap.Conn doesn't take contexts).
	conn.SetTimeout(c.cfg.Timeout)

	// Optional service bind.
	if c.cfg.BindDN != "" {
		if err := conn.Bind(c.cfg.BindDN, c.cfg.BindPassword); err != nil {
			return false, err
		}
	}

	filter := strings.ReplaceAll(c.cfg.UserFilter, "{username}", ldap.EscapeFilter(username))
	req := ldap.NewSearchRequest(
		c.cfg.BaseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		1, // size limit
		int(c.cfg.Timeout.Seconds()),
		false,
		filter,
		[]string{"dn"},
		nil,
	)

	res, err := conn.Search(req)
	if err != nil {
		return false, err
	}
	if len(res.Entries) == 0 {
		return false, nil
	}
	userDN := res.Entries[0].DN
	if userDN == "" {
		return false, errors.New("ldap: empty user dn")
	}

	// Bind as the user to validate password.
	if err := conn.Bind(userDN, password); err != nil {
		// Invalid credentials is not fatal to the system.
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return false, nil
		}
		return false, err
	}

	// Honor ctx (best effort).
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	return true, nil
}

func (c *Client) Mode() string {
	if c == nil {
		return "disabled"
	}
	return c.cfg.Mode
}

func (c *Client) Timeout() time.Duration {
	if c == nil || c.cfg.Timeout <= 0 {
		return 5 * time.Second
	}
	return c.cfg.Timeout
}
