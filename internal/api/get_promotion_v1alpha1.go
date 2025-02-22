package api

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
	"github.com/akuity/kargo/internal/api/types/v1alpha1"
	svcv1alpha1 "github.com/akuity/kargo/pkg/api/service/v1alpha1"
)

func (s *server) GetPromotion(
	ctx context.Context,
	req *connect.Request[svcv1alpha1.GetPromotionRequest],
) (*connect.Response[svcv1alpha1.GetPromotionResponse], error) {
	project := req.Msg.GetProject()
	if err := validateFieldNotEmpty("project", project); err != nil {
		return nil, err
	}

	name := req.Msg.GetName()
	if err := validateFieldNotEmpty("name", name); err != nil {
		return nil, err
	}

	if err := s.validateProjectExists(ctx, project); err != nil {
		return nil, err
	}

	var promotion kargoapi.Promotion
	if err := s.client.Get(ctx, client.ObjectKey{
		Namespace: project,
		Name:      name,
	}, &promotion); err != nil {
		if kubeerr.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound,
				errors.Errorf("promotion %q not found", name))
		}
		return nil, errors.Wrap(err, "get promotion")
	}
	return connect.NewResponse(&svcv1alpha1.GetPromotionResponse{
		Promotion: v1alpha1.ToPromotionProto(promotion),
	}), nil
}
