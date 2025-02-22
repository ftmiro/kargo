package api

import (
	"context"
	goerrors "errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
	typesv1alpha1 "github.com/akuity/kargo/internal/api/types/v1alpha1"
	"github.com/akuity/kargo/internal/kargo"
	svcv1alpha1 "github.com/akuity/kargo/pkg/api/service/v1alpha1"
	"github.com/akuity/kargo/pkg/api/v1alpha1"
)

// PromoteSubscribers creates a Promotion resources to transition all Stages
// immediately downstream from the specified Stage into the state represented by
// the specified Freight.
func (s *server) PromoteSubscribers(
	ctx context.Context,
	req *connect.Request[svcv1alpha1.PromoteSubscribersRequest],
) (*connect.Response[svcv1alpha1.PromoteSubscribersResponse], error) {
	project := req.Msg.GetProject()
	if err := validateFieldNotEmpty("project", project); err != nil {
		return nil, err
	}

	stageName := req.Msg.GetStage()
	if err := validateFieldNotEmpty("stage", stageName); err != nil {
		return nil, err
	}

	freightName := req.Msg.GetFreight()
	freightAlias := req.Msg.GetFreightAlias()
	if (freightName == "" && freightAlias == "") || (freightName != "" && freightAlias != "") {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.New("exactly one of freightName or freightAlias should not be empty"),
		)
	}

	if err := s.validateProjectExistsFn(ctx, project); err != nil {
		return nil, err
	}

	stage, err := s.getStageFn(
		ctx,
		s.client,
		types.NamespacedName{
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
			errors.Errorf(
				"Stage %q not found in namespace %q",
				stageName,
				project,
			),
		)
	}

	// Get the specified Freight, but only if it is verified in this Stage.
	// Merely being approved FOR this Stage is not enough. If Freight is only
	// approved FOR this Stage, that is because someone manually did that. This
	// does not speak to its suitability for promotion downstream. If a user
	// desires to promote Freight downstream that is not verified in this
	// Stage, then they should approve the Freight for the downstream Stage(s).
	// Expect a nil if the specified Freight is not found or doesn't meet these
	// conditions. Errors are indicative only of internal problems.
	freight, err := s.getFreightByNameOrAliasFn(
		ctx,
		s.client,
		project,
		freightName,
		freightAlias,
	)
	if err != nil {
		return nil, errors.Wrap(err, "get freight")
	}
	if freight == nil {
		if freightName != "" {
			err = fmt.Errorf("freight %q not found in namespace %q", freightName, project)
		} else {
			err = fmt.Errorf("freight with alias %q not found in namespace %q", freightAlias, project)
		}
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if !s.isFreightAvailableFn(
		freight,
		"",                  // approved for not considered
		[]string{stageName}, // verified in
	) {
		return nil, connect.NewError(
			connect.CodeInvalidArgument,
			errors.Errorf(
				"Freight %q is not available to Stage %q",
				freightName,
				stageName,
			),
		)
	}

	subscribers, err := s.findStageSubscribersFn(ctx, stage)
	if err != nil {
		return nil, errors.Wrap(err, "find stage subscribers")
	}
	if len(subscribers) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("stage %q has no subscribers", stageName))
	}

	for _, subscriber := range subscribers {
		if err := s.authorizeFn(
			ctx,
			"promote",
			kargoapi.GroupVersion.WithResource("stages"),
			"",
			types.NamespacedName{
				Namespace: subscriber.Namespace,
				Name:      subscriber.Name,
			},
		); err != nil {
			return nil, err
		}
	}

	promoteErrs := make([]error, 0, len(subscribers))
	createdPromos := make([]*v1alpha1.Promotion, 0, len(subscribers))
	for _, subscriber := range subscribers {
		newPromo := kargo.NewPromotion(subscriber, freight.Name)
		if err := s.createPromotionFn(ctx, &newPromo); err != nil {
			promoteErrs = append(promoteErrs, err)
			continue
		}
		createdPromos = append(createdPromos, typesv1alpha1.ToPromotionProto(newPromo))
	}

	res := connect.NewResponse(&svcv1alpha1.PromoteSubscribersResponse{
		Promotions: createdPromos,
	})

	if len(promoteErrs) > 0 {
		return res,
			connect.NewError(connect.CodeInternal, goerrors.Join(promoteErrs...))
	}

	return res, nil
}

// findStageSubscribers returns a list of Stages that are subscribed to the given Stage
// TODO: this could be powered by an index.
func (s *server) findStageSubscribers(ctx context.Context, stage *kargoapi.Stage) ([]kargoapi.Stage, error) {
	var allStages kargoapi.StageList
	if err := s.client.List(ctx, &allStages, client.InNamespace(stage.Namespace)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var subscribers []kargoapi.Stage
	for _, s := range allStages.Items {
		s := s
		if s.Spec.Subscriptions == nil {
			continue
		}
		for _, upstream := range s.Spec.Subscriptions.UpstreamStages {
			if upstream.Name != stage.Name {
				continue
			}
			subscribers = append(subscribers, s)
		}
	}
	return subscribers, nil
}
