package api

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
	"github.com/akuity/kargo/internal/kubeclient"
	svcv1alpha1 "github.com/akuity/kargo/pkg/api/service/v1alpha1"
)

func (s *server) ApproveFreight(
	ctx context.Context,
	req *connect.Request[svcv1alpha1.ApproveFreightRequest],
) (*connect.Response[svcv1alpha1.ApproveFreightResponse], error) {
	project := req.Msg.GetProject()
	if err := validateFieldNotEmpty("project", project); err != nil {
		return nil, err
	}

	name := req.Msg.GetName()
	alias := req.Msg.GetAlias()
	if (name == "" && alias == "") ||
		(name != "" && alias != "") {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("exactly one of name or alias should not be empty"),
		)
	}

	stageName := req.Msg.GetStage()
	if err := validateFieldNotEmpty("stage", stageName); err != nil {
		return nil, err
	}

	if err := s.validateProjectExistsFn(ctx, project); err != nil {
		return nil, err
	}

	freight, err := s.getFreightByNameOrAliasFn(
		ctx,
		s.client,
		project,
		name,
		alias,
	)
	if err != nil {
		return nil, errors.Wrap(err, "get freight")
	}
	if freight == nil {
		if name != "" {
			err = fmt.Errorf("freight %q not found in namespace %q", name, project)
		} else {
			err = fmt.Errorf("freight with alias %q not found in namespace %q", alias, project)
		}
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	stage, err := s.getStageFn(
		ctx,
		s.client,
		client.ObjectKey{
			Namespace: project,
			Name:      stageName,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "get stage")
	}
	if stage == nil {
		return nil, connect.NewError(
			connect.CodeNotFound,
			errors.Errorf("Stage %q not found in namespace %q", stageName, project),
		)
	}

	if err := s.authorizeFn(
		ctx,
		"promote",
		schema.GroupVersionResource{
			Group:    kargoapi.GroupVersion.Group,
			Version:  kargoapi.GroupVersion.Version,
			Resource: "stages",
		},
		"",
		types.NamespacedName{
			Namespace: project,
			Name:      stageName,
		},
	); err != nil {
		return nil, err
	}

	newStatus := *freight.Status.DeepCopy()
	if newStatus.ApprovedFor == nil {
		newStatus.ApprovedFor = map[string]kargoapi.ApprovedStage{}
	}

	if _, ok := newStatus.ApprovedFor[stageName]; ok {
		return &connect.Response[svcv1alpha1.ApproveFreightResponse]{}, nil
	}

	newStatus.ApprovedFor[stageName] = kargoapi.ApprovedStage{}

	if err := s.patchFreightStatusFn(ctx, freight, newStatus); err != nil {
		return nil, errors.Wrap(err, "patch status")
	}

	return &connect.Response[svcv1alpha1.ApproveFreightResponse]{}, nil
}

func (s *server) patchFreightStatus(
	ctx context.Context,
	freight *kargoapi.Freight,
	newStatus kargoapi.FreightStatus,
) error {
	err := kubeclient.PatchStatus(
		ctx,
		s.client,
		freight,
		func(status *kargoapi.FreightStatus) {
			*status = newStatus
		},
	)
	return errors.Wrapf(
		err,
		"error patching Freight %q status in namespace %q",
		freight.Name,
		freight.Namespace,
	)
}
