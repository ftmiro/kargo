package api

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	kubeerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
	typesv1alpha1 "github.com/akuity/kargo/internal/api/types/v1alpha1"
	svcv1alpha1 "github.com/akuity/kargo/pkg/api/service/v1alpha1"
)

func (s *server) WatchPromotion(
	ctx context.Context,
	req *connect.Request[svcv1alpha1.WatchPromotionRequest],
	stream *connect.ServerStream[svcv1alpha1.WatchPromotionResponse],
) error {
	project := req.Msg.GetProject()
	if err := validateFieldNotEmpty("project", project); err != nil {
		return err
	}

	name := req.Msg.GetName()
	if err := validateFieldNotEmpty("name", name); err != nil {
		return err
	}

	if err := s.validateProjectExists(ctx, project); err != nil {
		return err
	}

	if err := s.client.Get(ctx, client.ObjectKey{
		Namespace: project,
		Name:      name,
	}, &kargoapi.Promotion{}); err != nil {
		if kubeerr.IsNotFound(err) {
			return connect.NewError(connect.CodeNotFound, err)
		}
		return errors.Wrap(err, "get promotion")
	}

	opts := metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(metav1.ObjectNameField, name).String(),
	}
	w, err := s.client.Watch(ctx, &kargoapi.Promotion{}, project, opts)
	if err != nil {
		return errors.Wrap(err, "watch promotion")
	}
	defer w.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e, ok := <-w.ResultChan():
			if !ok {
				return nil
			}
			u, ok := e.Object.(*unstructured.Unstructured)
			if !ok {
				return errors.Errorf("unexpected object type %T", e.Object)
			}
			var promotion *kargoapi.Promotion
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &promotion); err != nil {
				return errors.Wrap(err, "from unstructured")
			}
			if err := stream.Send(&svcv1alpha1.WatchPromotionResponse{
				Promotion: typesv1alpha1.ToPromotionProto(*promotion),
				Type:      string(e.Type),
			}); err != nil {
				return errors.Wrap(err, "send response")
			}
		}
	}
}
