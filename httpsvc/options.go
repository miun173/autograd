package httpsvc

import "github.com/miun173/autograd/usecase"

// Option ..
type Option func(*Server)

// WithExampleUsecase ..
func WithExampleUsecase(ex usecase.ExampleUsecase) Option {
	return func(s *Server) {
		s.exampleUsecase = ex
	}
}
