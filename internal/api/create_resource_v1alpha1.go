package api

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigyaml "sigs.k8s.io/yaml"

	svcv1alpha1 "github.com/akuity/kargo/pkg/api/service/v1alpha1"
)

func (s *server) CreateResource(
	ctx context.Context,
	req *connect.Request[svcv1alpha1.CreateResourceRequest],
) (*connect.Response[svcv1alpha1.CreateResourceResponse], error) {
	projects, otherResources, err := splitYAML(req.Msg.GetManifest())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.Wrap(err, "parse manifest"))
	}
	resources := append(projects, otherResources...)
	res := make([]*svcv1alpha1.CreateResourceResult, 0, len(resources))
	for _, r := range resources {
		resource := r // Avoid implicit memory aliasing
		res = append(res, s.createResource(ctx, &resource))
	}
	return &connect.Response[svcv1alpha1.CreateResourceResponse]{
		Msg: &svcv1alpha1.CreateResourceResponse{
			Results: res,
		},
	}, nil
}

func (s *server) createResource(
	ctx context.Context,
	obj *unstructured.Unstructured,
) *svcv1alpha1.CreateResourceResult {
	if err := s.client.Create(ctx, obj); err != nil {
		return &svcv1alpha1.CreateResourceResult{
			Result: &svcv1alpha1.CreateResourceResult_Error{
				Error: errors.Wrap(err, "create resource").Error(),
			},
		}
	}

	createdManifest, err := sigyaml.Marshal(obj)
	if err != nil {
		return &svcv1alpha1.CreateResourceResult{
			Result: &svcv1alpha1.CreateResourceResult_Error{
				Error: errors.Wrap(err, "marshal created manifest").Error(),
			},
		}
	}
	return &svcv1alpha1.CreateResourceResult{
		Result: &svcv1alpha1.CreateResourceResult_CreatedResourceManifest{
			CreatedResourceManifest: createdManifest,
		},
	}
}
