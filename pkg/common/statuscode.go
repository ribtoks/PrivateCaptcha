package common

import "strconv"

type StatusCode int

const (
	// common errors
	StatusOK             StatusCode = 1000
	StatusFailure        StatusCode = 1001
	StatusNotImplemented StatusCode = 1002
	StatusApiDeprecated  StatusCode = 1003
	// organization errors
	StatusOrgNameEmptyError          StatusCode = 1100
	StatusOrgNameTooLongError        StatusCode = 1101
	StatusOrgNameInvalidSymbolsError StatusCode = 1102
	StatusOrgExistsError             StatusCode = 1103
	StatusOrgLimitError              StatusCode = 1104
	StatusOrgNotFoundError           StatusCode = 1105
	StatusOrgPermissionsError        StatusCode = 1106
)

func (sc StatusCode) Success() bool {
	return sc == StatusOK
}

func (sc StatusCode) String() string {
	switch sc {
	case StatusOK:
		return "OK"
	case StatusFailure:
		return "Failure"
	case StatusNotImplemented:
		return "Not implemented"
	case StatusApiDeprecated:
		return "API is deprecated"
	case StatusOrgNameEmptyError:
		return "Name cannot be empty."
	case StatusOrgNameTooLongError:
		return "Name is too long."
	case StatusOrgNameInvalidSymbolsError:
		return "Organization name contains invalid characters."
	case StatusOrgExistsError:
		return "Organization with this name already exists."
	case StatusOrgLimitError:
		return "Organizations limit reached on your current plan, please upgrade to create more."
	case StatusOrgNotFoundError:
		return "Requested organization does not seem to exist."
	case StatusOrgPermissionsError:
		return "You do not have permissions to access this organization."
	default:
		return strconv.Itoa(int(sc))
	}
}
