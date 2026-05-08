package auth

func HasRole(claims *Claims, role string) bool {
	if claims == nil || role == "" {
		return false
	}
	for _, value := range claims.Roles {
		if value == role {
			return true
		}
	}
	return false
}

func HasAnyRole(claims *Claims, roles ...string) bool {
	for _, role := range roles {
		if HasRole(claims, role) {
			return true
		}
	}
	return false
}

func HasScope(claims *Claims, scope string) bool {
	if claims == nil || scope == "" {
		return false
	}
	for _, value := range claims.Scopes {
		if value == scope {
			return true
		}
	}
	return false
}

func HasAnyScope(claims *Claims, scopes ...string) bool {
	for _, scope := range scopes {
		if HasScope(claims, scope) {
			return true
		}
	}
	return false
}
