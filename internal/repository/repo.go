package repository

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/AndrivA89/knowledge-manager/internal/domain"
)

type NodeRepository struct {
	driver neo4j.DriverWithContext
}

func NewNodeRepository(driver neo4j.DriverWithContext) *NodeRepository {
	return &NodeRepository{
		driver: driver,
	}
}

func (r *NodeRepository) CreateNode(ctx context.Context, node *domain.Node) (string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func(session neo4j.SessionWithContext, ctx context.Context) {
		err := session.Close(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}(session, ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		node.CreatedAt = time.Now()
		node.UpdatedAt = time.Now()

		query := `
			CREATE (n:Node {
				id: randomUUID(),
				title: $title,
				content: $content,
				type: $type,
				created_at: datetime($created_at),
				updated_at: datetime($updated_at)
			})
			SET n:` + string(node.Type) + `
			FOREACH (tag IN $tags | MERGE (t:Tag {name: tag}) MERGE (n)-[:HAS_TAG]->(t))
			RETURN n.id as id
		`

		params := map[string]interface{}{
			"title":      node.Title,
			"content":    node.Content,
			"type":       string(node.Type),
			"created_at": node.CreatedAt.Format(time.RFC3339),
			"updated_at": node.UpdatedAt.Format(time.RFC3339),
			"tags":       node.Tags,
		}

		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		record, err := result.Single(ctx)
		if err != nil {
			return nil, err
		}

		id, _ := record.Get("id")
		return id, nil
	})

	if err != nil {
		return "", err
	}

	return result.(string), nil
}

func (r *NodeRepository) CreateRelationship(ctx context.Context, rel *domain.Relationship) ([]string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func(session neo4j.SessionWithContext, ctx context.Context) {
		err := session.Close(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}(session, ctx)

	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		rel.CreatedAt = time.Now()

		query := `
			MATCH (source:Node {id: $source_id})
			UNWIND $target_ids AS tID
			MATCH (target:Node {id: tID})
			CREATE (source)-[r:` + string(rel.Type) + ` {
				id: randomUUID(),
				description: $description,
				created_at: datetime($created_at)
			}]->(target)
			RETURN collect(r.id) as ids
		`

		params := map[string]interface{}{
			"source_id":   rel.SourceID,
			"target_ids":  rel.TargetIDs,
			"description": rel.Description,
			"created_at":  rel.CreatedAt.Format(time.RFC3339),
		}

		cyRes, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		record, err := cyRes.Single(ctx)
		if err != nil {
			return nil, err
		}

		idsVal, _ := record.Get("ids")
		idsSlice, ok := idsVal.([]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected type for 'ids' column")
		}

		var relIDs []string
		for _, v := range idsSlice {
			if s, ok := v.(string); ok {
				relIDs = append(relIDs, s)
			}
		}
		return relIDs, nil
	})

	if err != nil {
		return nil, err
	}

	return result.([]string), nil
}

func (r *NodeRepository) GetNodeByID(ctx context.Context, id string) (*domain.Node, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer func(session neo4j.SessionWithContext, ctx context.Context) {
		err := session.Close(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}(session, ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (n:Node {id: $id})
			OPTIONAL MATCH (n)-[:HAS_TAG]->(t:Tag)
			RETURN n.id as id, n.title as title, n.content as content, n.type as type, 
				   n.created_at as created_at, n.updated_at as updated_at, collect(t.name) as tags
		`

		params := map[string]interface{}{
			"id": id,
		}

		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		record, err := result.Single(ctx)
		if err != nil {
			return nil, err
		}

		node := &domain.Node{}

		idVal, _ := record.Get("id")
		node.ID = idVal.(string)

		titleVal, _ := record.Get("title")
		node.Title = titleVal.(string)

		contentVal, _ := record.Get("content")
		node.Content = contentVal.(string)

		nodeType, _ := record.Get("type")
		node.Type = domain.NodeType(nodeType.(string))

		createdAt, _ := record.Get("created_at")
		updatedAt, _ := record.Get("updated_at")

		node.CreatedAt = createdAt.(time.Time)
		node.UpdatedAt = updatedAt.(time.Time)

		tags, _ := record.Get("tags")
		for _, tag := range tags.([]interface{}) {
			node.Tags = append(node.Tags, tag.(string))
		}

		return node, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*domain.Node), nil
}

func (r *NodeRepository) UpdateNode(ctx context.Context, node *domain.Node) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func(session neo4j.SessionWithContext, ctx context.Context) {
		if err := session.Close(ctx); err != nil {
			log.Fatal(err)
		}
	}(session, ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		node.UpdatedAt = time.Now()

		query := `
			MATCH (n:Node {id: $id})
			SET n.title = $title,
			    n.content = $content,
			    n.type = $type,
			    n.updated_at = datetime($updated_at)
			WITH n
			OPTIONAL MATCH (n)-[r:HAS_TAG]->(:Tag)
			DELETE r
			WITH DISTINCT n
			LIMIT 1
			FOREACH (tag IN $tags |
				MERGE (t:Tag {name: tag})
				MERGE (n)-[:HAS_TAG]->(t)
			)
			RETURN n
		`

		params := map[string]interface{}{
			"id":         node.ID,
			"title":      node.Title,
			"content":    node.Content,
			"type":       string(node.Type),
			"updated_at": node.UpdatedAt.Format(time.RFC3339),
			"tags":       node.Tags,
		}

		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		_, err = result.Single(ctx)
		return nil, err
	})

	return err
}

func (r *NodeRepository) DeleteNode(ctx context.Context, id string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func(session neo4j.SessionWithContext, ctx context.Context) {
		err := session.Close(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}(session, ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (n:Node {id: $id})
			DETACH DELETE n
		`

		params := map[string]interface{}{
			"id": id,
		}

		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		_, err = result.Consume(ctx)
		return nil, err
	})

	return err
}

func (r *NodeRepository) DeleteRelationship(ctx context.Context, relationshipID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer func(session neo4j.SessionWithContext, ctx context.Context) {
		err := session.Close(ctx)
		if err != nil {
			log.Fatal(err)
		}
	}(session, ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH ()-[r {id: $id}]-()
			DELETE r
		`
		params := map[string]interface{}{
			"id": relationshipID,
		}
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}

		_, err = result.Consume(ctx)
		return nil, err
	})

	return err
}
