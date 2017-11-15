package actions

import (
	"github.com/sensu/sensu-go/backend/authorization"
	"github.com/sensu/sensu-go/backend/store"
	"github.com/sensu/sensu-go/types"
	"golang.org/x/net/context"
)

// EventController expose actions in which a viewer can perform.
type EventController struct {
	Store  store.EventStore
	Policy authorization.EventPolicy
}

// NewEventController returns new EventController
func NewEventController(store store.EventStore) EventController {
	return EventController{
		Store:  store,
		Policy: authorization.Events,
	}
}

// Query returns resources available to the viewer filter by given params.
func (a EventController) Query(ctx context.Context, params QueryParams) ([]*types.Event, error) {
	var results []*types.Event

	entityID := params["entity"]
	checkName := params["check"]

	// Fetch from store
	var serr error
	if entityID != "" && checkName != "" {
		var result *types.Event
		result, serr = a.Store.GetEventByEntityCheck(ctx, entityID, checkName)
		if result != nil {
			results = append(results, result)
		}
	} else if entityID != "" {
		results, serr = a.Store.GetEventsByEntity(ctx, entityID)
	} else {
		results, serr = a.Store.GetEvents(ctx)
	}

	if serr != nil {
		return nil, NewError(InternalErr, serr)
	}

	// Filter out those resources the viewer does not have access to view.
	abilities := a.Policy.WithContext(ctx)
	for i := 0; i < len(results); i++ {
		if !abilities.CanRead(results[i]) {
			results = append(results[:i], results[i+1:]...)
			i--
		}
	}

	return results, nil
}

// Find returns resource associated with given parameters if available to the
// viewer.
func (a EventController) Find(ctx context.Context, params QueryParams) (*types.Event, error) {
	// Find (for events) requires both an entity and check
	if params["entity"] == "" || params["check"] == "" {
		return nil, NewErrorf(InvalidArgument, "Find() requires both an entity and a check")
	}

	result, err := a.Store.GetEventByEntityCheck(ctx, params["entity"], params["check"])
	if err != nil {
		return nil, err
	}

	// Verify user has permission to view
	abilities := a.Policy.WithContext(ctx)
	if result != nil && abilities.CanRead(result) {
		return result, nil
	}

	return nil, NewErrorf(NotFound)
}