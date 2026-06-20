package mail

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"strings"

	"gorp/internal/domain"
	"gorp/internal/record"
)

func SignupPrepare(env *record.Env, partnerIDs []int64, signupType string) error {
	if env == nil {
		return fmt.Errorf("signup requires env")
	}
	partnerIDs = uniqueIDs(partnerIDs)
	if len(partnerIDs) == 0 {
		return fmt.Errorf("signup requires partner ids")
	}
	signupType = strings.TrimSpace(signupType)
	if signupType == "" {
		signupType = "signup"
	}
	return messageSystemEnv(env).Model("res.partner").Browse(partnerIDs...).Write(map[string]any{"signup_type": signupType})
}

func SignupCancel(env *record.Env, partnerIDs []int64) error {
	if env == nil {
		return fmt.Errorf("signup requires env")
	}
	partnerIDs = uniqueIDs(partnerIDs)
	if len(partnerIDs) == 0 {
		return fmt.Errorf("signup requires partner ids")
	}
	return messageSystemEnv(env).Model("res.partner").Browse(partnerIDs...).Write(map[string]any{"signup_type": ""})
}

func SignupAuthParams(env *record.Env, partnerIDs []int64) (map[int64]map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("signup requires env")
	}
	partnerIDs = uniqueIDs(partnerIDs)
	out := make(map[int64]map[string]any, len(partnerIDs))
	if len(partnerIDs) == 0 {
		return out, nil
	}
	systemEnv := messageSystemEnv(env)
	usersByPartner, err := signupUsersByPartner(systemEnv, partnerIDs)
	if err != nil {
		return nil, err
	}
	allowSignup := configParameter(systemEnv, "auth_signup.invitation_scope") == "b2c"
	for _, partnerID := range partnerIDs {
		params := map[string]any{}
		if login := usersByPartner[partnerID]; login != "" {
			params["auth_login"] = login
		} else if allowSignup {
			if err := SignupPrepare(systemEnv, []int64{partnerID}, "signup"); err != nil {
				return nil, err
			}
			params["auth_signup_token"] = signupToken(systemEnv, partnerID, "signup")
		}
		out[partnerID] = params
	}
	return out, nil
}

func signupUsersByPartner(env *record.Env, partnerIDs []int64) (map[int64]string, error) {
	found, err := env.Model("res.users").Search(domain.Cond("partner_id", "in", partnerIDs))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("partner_id", "login")
	if err != nil {
		return nil, err
	}
	out := map[int64]string{}
	for _, row := range rows {
		partnerID := int64FromAny(row["partner_id"])
		if partnerID != 0 && out[partnerID] == "" {
			out[partnerID] = strings.TrimSpace(stringFromAny(row["login"]))
		}
	}
	return out, nil
}

func signupToken(env *record.Env, partnerID int64, signupType string) string {
	secret := configParameter(env, "database.secret")
	if secret == "" {
		secret = "gorp"
	}
	payload := fmt.Sprintf("('signup', ('%s', %d, '%s'))", portalDBName(env), partnerID, signupType)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}
