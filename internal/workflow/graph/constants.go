package graph

// Node type strings (Block.Metadata.ID). Aligned with sim's executor/handlers set.
const (
	TypeTrigger        = "trigger"
	TypeAgent          = "agent"
	TypeTool           = "tool"
	TypeFunction       = "function"
	TypeAPI            = "api"
	TypeCondition      = "condition"
	TypeRouter         = "router"
	TypeResponse       = "response"
	TypeWait           = "wait"
	TypeHumanInTheLoop = "human-in-the-loop"
	TypeKnowledge      = "knowledge"
	TypeEvaluator      = "evaluator"
	TypeVariables      = "variables"
	TypeWorkflow       = "workflow" // sub-workflow
)

// Well-known source handles. Aligned with sim's edge semantics.
const (
	HandleSource           = "source" // default success edge
	HandleError            = "error"  // taken when source block errors
	HandleConditionPrefix  = "condition-"
	HandleRouterPrefix     = "router-"
	HandleLoopContinue     = "loop_continue"
	HandleLoopExit         = "loop_exit"
	HandleParallelContinue = "parallel_continue"
	HandleParallelExit     = "parallel_exit"
)

// Loop type strings.
const (
	LoopFor     = "for"
	LoopForEach = "forEach"
	LoopWhile   = "while"
	LoopDoWhile = "doWhile"
)

// Parallel type strings.
const (
	ParallelCount      = "count"
	ParallelCollection = "collection"
)
