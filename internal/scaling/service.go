package scaling

// Service represents the common interface for all scaling services
type Service interface {
    Start() error
    Stop() error
}