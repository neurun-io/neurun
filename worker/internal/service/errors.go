package service

type executionError struct {
	message   string
	category  string
	retryable bool
}

func (e executionError) Error() string {
	return e.message
}

func permanent(message string) executionError {
	return executionError{message: message, category: "permanent", retryable: false}
}

func infrastructure(message string) executionError {
	return executionError{message: message, category: "infrastructure", retryable: true}
}
