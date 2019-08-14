package interpreter

import (
	"fmt"
	goRuntime "runtime"

	"github.com/raviqqe/hamt"

	"github.com/dapperlabs/bamboo-node/pkg/language/runtime/activations"
	"github.com/dapperlabs/bamboo-node/pkg/language/runtime/ast"
	"github.com/dapperlabs/bamboo-node/pkg/language/runtime/common"
	"github.com/dapperlabs/bamboo-node/pkg/language/runtime/errors"
	"github.com/dapperlabs/bamboo-node/pkg/language/runtime/sema"
	. "github.com/dapperlabs/bamboo-node/pkg/language/runtime/trampoline"
)

type loopBreak struct{}
type loopContinue struct{}
type functionReturn struct {
	Value
}

// Visit-methods for statement which return a non-nil value
// are treated like they are returning a value.

type Interpreter struct {
	Program     *ast.Program
	activations *activations.Activations
	Globals     map[string]*Variable
}

func NewInterpreter(program *ast.Program) *Interpreter {
	return &Interpreter{
		Program:     program,
		activations: &activations.Activations{},
		Globals:     map[string]*Variable{},
	}
}

func (interpreter *Interpreter) findVariable(name string) *Variable {
	return interpreter.activations.Find(name).(*Variable)
}

func (interpreter *Interpreter) setVariable(name string, variable *Variable) {
	interpreter.activations.Set(name, variable)
}

func (interpreter *Interpreter) Interpret() (err error) {
	// recover internal panics and return them as an error
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			// don't recover Go errors
			err, ok = r.(goRuntime.Error)
			if ok {
				panic(err)
			}
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	Run(More(func() Trampoline {
		return interpreter.visitProgramDeclarations()
	}))

	return nil
}

func (interpreter *Interpreter) visitProgramDeclarations() Trampoline {
	return interpreter.visitGlobalDeclarations(interpreter.Program.Declarations)
}

func (interpreter *Interpreter) visitGlobalDeclarations(declarations []ast.Declaration) Trampoline {
	count := len(declarations)

	// no declarations? stop
	if count == 0 {
		// NOTE: no result, so it does *not* act like a return-statement
		return Done{}
	}

	// interpret the first declaration, then the remaining ones
	return interpreter.visitGlobalDeclaration(declarations[0]).
		FlatMap(func(_ interface{}) Trampoline {
			return interpreter.visitGlobalDeclarations(declarations[1:])
		})
}

// visitGlobalDeclaration firsts interprets the global declaration,
// then finds the declaration and adds it to the globals
func (interpreter *Interpreter) visitGlobalDeclaration(declaration ast.Declaration) Trampoline {
	return declaration.Accept(interpreter).(Trampoline).
		Then(func(_ interface{}) {
			interpreter.declareGlobal(declaration)
		})
}

func (interpreter *Interpreter) declareGlobal(declaration ast.Declaration) {
	name := declaration.DeclarationName()
	// NOTE: semantic analysis already checked possible invalid redeclaration
	interpreter.Globals[name] = interpreter.findVariable(name)
}

func (interpreter *Interpreter) Invoke(functionName string, inputs ...interface{}) (value Value, err error) {
	variable, ok := interpreter.Globals[functionName]
	if !ok {
		return nil, &NotDeclaredError{
			ExpectedKind: common.DeclarationKindFunction,
			Name:         functionName,
		}
	}

	variableValue := variable.Value

	function, ok := variableValue.(FunctionValue)
	if !ok {
		return nil, &NotCallableError{
			Value: variableValue,
		}
	}

	arguments, err := ToValues(inputs)
	if err != nil {
		return nil, err
	}

	// recover internal panics and return them as an error
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			// don't recover Go errors
			err, ok = r.(goRuntime.Error)
			if ok {
				panic(err)
			}
			err, ok = r.(error)
			if !ok {
				err = fmt.Errorf("%v", r)
			}
		}
	}()

	// ensures the invocation's argument count matches the function's parameter count

	parameterCount := function.parameterCount()
	argumentCount := len(arguments)

	if argumentCount != parameterCount {
		return nil, &ArgumentCountError{
			ParameterCount: parameterCount,
			ArgumentCount:  argumentCount,
		}
	}

	result := Run(function.invoke(interpreter, arguments))
	if result == nil {
		return nil, nil
	}
	return result.(Value), nil
}

func (interpreter *Interpreter) InvokeExportable(
	functionName string,
	inputs ...interface{},
) (
	value ExportableValue,
	err error,
) {
	result, err := interpreter.Invoke(functionName, inputs...)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, nil
	}

	return result.(ExportableValue), nil
}

func (interpreter *Interpreter) VisitProgram(program *ast.Program) ast.Repr {
	panic(&errors.UnreachableError{})
}

func (interpreter *Interpreter) VisitFunctionDeclaration(declaration *ast.FunctionDeclaration) ast.Repr {

	// lexical scope: variables in functions are bound to what is visible at declaration time
	lexicalScope := interpreter.activations.CurrentOrNew()

	// make the function itself available inside the function
	variable := &Variable{}

	lexicalScope = lexicalScope.
		Insert(activations.StringKey(declaration.Identifier), variable)

	functionExpression := declaration.ToExpression()
	variable.Value = newInterpretedFunction(functionExpression, lexicalScope)

	// declare the function in the current scope
	interpreter.setVariable(declaration.Identifier, variable)

	// NOTE: no result, so it does *not* act like a return-statement
	return Done{}
}

func (interpreter *Interpreter) ImportFunction(name string, function HostFunctionValue) {
	interpreter.declareVariable(name, function)
}

func (interpreter *Interpreter) VisitBlock(block *ast.Block) ast.Repr {
	// block scope: each block gets an activation record
	interpreter.activations.PushCurrent()

	return interpreter.visitStatements(block.Statements).
		Then(func(_ interface{}) {
			interpreter.activations.Pop()
		})
}

func (interpreter *Interpreter) visitStatements(statements []ast.Statement) Trampoline {
	count := len(statements)

	// no statements? stop
	if count == 0 {
		// NOTE: no result, so it does *not* act like a return-statement
		return Done{}
	}

	// interpret the first statement, then the remaining ones
	return statements[0].Accept(interpreter).(Trampoline).
		FlatMap(func(returnValue interface{}) Trampoline {
			if returnValue != nil {
				return Done{Result: returnValue}
			}
			return interpreter.visitStatements(statements[1:])
		})
}

func (interpreter *Interpreter) VisitFunctionBlock(functionBlock *ast.FunctionBlock) ast.Repr {
	// NOTE: see visitFunctionBlock
	panic(&errors.UnreachableError{})
}

func (interpreter *Interpreter) visitFunctionBlock(functionBlock *ast.FunctionBlock, returnType ast.Type) Trampoline {

	// block scope: each function block gets an activation record
	interpreter.activations.PushCurrent()

	beforeStatements, rewrittenPostConditions :=
		interpreter.rewritePostConditions(functionBlock)

	return interpreter.visitStatements(beforeStatements).
		FlatMap(func(_ interface{}) Trampoline {
			return interpreter.visitConditions(functionBlock.PreConditions)
		}).
		FlatMap(func(_ interface{}) Trampoline {
			// NOTE: not interpreting block as it enters a new scope
			// and post-conditions need to be able to refer to block's declarations
			return interpreter.visitStatements(functionBlock.Block.Statements).
				FlatMap(func(blockResult interface{}) Trampoline {

					var resultValue Value
					if blockResult == nil {
						resultValue = VoidValue{}
					} else {
						resultValue = blockResult.(functionReturn).Value
					}

					// if there is a return type, declare the constant `result`
					// which has the return value

					// TODO: improve
					if ty, ok := returnType.(*ast.NominalType); ok && ty.Identifier != "" {
						interpreter.declareVariable(sema.ResultIdentifier, resultValue)
					}

					return interpreter.visitConditions(rewrittenPostConditions).
						Map(func(_ interface{}) interface{} {
							return resultValue
						})
				})
		}).
		Then(func(_ interface{}) {
			interpreter.activations.Pop()
		})
}

func (interpreter *Interpreter) rewritePostConditions(functionBlock *ast.FunctionBlock) (
	beforeStatements []ast.Statement,
	rewrittenPostConditions []*ast.Condition,
) {
	beforeExtractor := NewBeforeExtractor()

	rewrittenPostConditions = make([]*ast.Condition, len(functionBlock.PostConditions))

	for i, postCondition := range functionBlock.PostConditions {
		extraction := beforeExtractor.ExtractBefore(postCondition.Expression)

		for _, extractedExpression := range extraction.ExtractedExpressions {

			beforeStatements = append(beforeStatements,
				&ast.VariableDeclaration{
					Identifier: extractedExpression.Identifier,
					Value:      extractedExpression.Expression,
				},
			)
		}

		// copy condition and set expression to rewritten one
		newPostCondition := *postCondition
		newPostCondition.Expression = extraction.RewrittenExpression

		rewrittenPostConditions[i] = &newPostCondition
	}

	return beforeStatements, rewrittenPostConditions
}

func (interpreter *Interpreter) visitConditions(conditions []*ast.Condition) Trampoline {
	count := len(conditions)

	// no conditions? stop
	if count == 0 {
		return Done{}
	}

	// interpret the first condition, then the remaining ones
	condition := conditions[0]
	return condition.Accept(interpreter).(Trampoline).
		FlatMap(func(value interface{}) Trampoline {
			result := value.(BoolValue)

			if !result {
				panic(&ConditionError{
					ConditionKind: condition.Kind,
					StartPos:      condition.Expression.StartPosition(),
					EndPos:        condition.Expression.EndPosition(),
				})
			}

			return interpreter.visitConditions(conditions[1:])
		})
}

func (interpreter *Interpreter) VisitCondition(condition *ast.Condition) ast.Repr {
	return condition.Expression.Accept(interpreter)
}

func (interpreter *Interpreter) VisitReturnStatement(statement *ast.ReturnStatement) ast.Repr {
	// NOTE: returning result

	if statement.Expression == nil {
		return Done{Result: functionReturn{VoidValue{}}}
	}

	return statement.Expression.Accept(interpreter).(Trampoline).
		Map(func(value interface{}) interface{} {
			return functionReturn{value.(Value)}
		})
}

func (interpreter *Interpreter) VisitBreakStatement(statement *ast.BreakStatement) ast.Repr {
	return Done{Result: loopBreak{}}
}

func (interpreter *Interpreter) VisitContinueStatement(statement *ast.ContinueStatement) ast.Repr {
	return Done{Result: loopContinue{}}
}

func (interpreter *Interpreter) VisitIfStatement(statement *ast.IfStatement) ast.Repr {
	return statement.Test.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			value := result.(BoolValue)
			if value {
				return statement.Then.Accept(interpreter).(Trampoline)
			} else if statement.Else != nil {
				return statement.Else.Accept(interpreter).(Trampoline)
			}

			// NOTE: no result, so it does *not* act like a return-statement
			return Done{}
		})
}

func (interpreter *Interpreter) VisitWhileStatement(statement *ast.WhileStatement) ast.Repr {
	return statement.Test.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			value := result.(BoolValue)
			if !value {
				return Done{}
			}

			return statement.Block.Accept(interpreter).(Trampoline).
				FlatMap(func(value interface{}) Trampoline {
					if _, ok := value.(loopBreak); ok {
						return Done{}
					} else if _, ok := value.(loopContinue); ok {
						// NO-OP
					} else if functionReturn, ok := value.(functionReturn); ok {
						return Done{Result: functionReturn}
					}

					// recurse
					return statement.Accept(interpreter).(Trampoline)
				})
		})
}

// VisitVariableDeclaration first visits the declaration's value,
// then declares the variable with the name bound to the value
func (interpreter *Interpreter) VisitVariableDeclaration(declaration *ast.VariableDeclaration) ast.Repr {
	return declaration.Value.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			value := result.(Value)

			interpreter.declareVariable(declaration.Identifier, value)

			// NOTE: ignore result, so it does *not* act like a return-statement
			return Done{}
		})
}

func (interpreter *Interpreter) declareVariable(identifier string, value Value) {
	// NOTE: semantic analysis already checked possible invalid redeclaration
	interpreter.setVariable(identifier, &Variable{
		Value: value,
	})
}

func (interpreter *Interpreter) VisitAssignment(assignment *ast.AssignmentStatement) ast.Repr {
	return assignment.Value.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			value := result.(Value)
			return interpreter.visitAssignmentValue(assignment, value)
		})
}

func (interpreter *Interpreter) visitAssignmentValue(assignment *ast.AssignmentStatement, value Value) Trampoline {
	switch target := assignment.Target.(type) {
	case *ast.IdentifierExpression:
		interpreter.visitIdentifierExpressionAssignment(target, value)
		// NOTE: no result, so it does *not* act like a return-statement
		return Done{}

	case *ast.IndexExpression:
		return interpreter.visitIndexExpressionAssignment(target, value)

	case *ast.MemberExpression:
		return interpreter.visitMemberExpressionAssignment(target, value)
	}

	panic(&errors.UnreachableError{})
}

func (interpreter *Interpreter) visitIdentifierExpressionAssignment(target *ast.IdentifierExpression, value Value) {
	variable := interpreter.findVariable(target.Identifier)
	variable.Value = value
}

func (interpreter *Interpreter) visitIndexExpressionAssignment(target *ast.IndexExpression, value Value) Trampoline {
	return target.Expression.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			array := result.(ArrayValue)

			return target.Index.Accept(interpreter).(Trampoline).
				FlatMap(func(result interface{}) Trampoline {
					index := result.(IntegerValue)
					array[index.IntValue()] = value

					// NOTE: no result, so it does *not* act like a return-statement
					return Done{}
				})
		})
}

func (interpreter *Interpreter) visitMemberExpressionAssignment(target *ast.MemberExpression, value Value) Trampoline {
	return target.Expression.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			structure := result.(*StructureValue)

			structure.Members[target.Identifier] = value

			// NOTE: no result, so it does *not* act like a return-statement
			return Done{}
		})
}

func (interpreter *Interpreter) VisitIdentifierExpression(expression *ast.IdentifierExpression) ast.Repr {
	variable := interpreter.findVariable(expression.Identifier)
	return Done{Result: variable.Value}
}

// visitBinaryOperation interprets the left-hand side and the right-hand side and returns
// the result in a TupleValue
func (interpreter *Interpreter) visitBinaryOperation(expr *ast.BinaryExpression) Trampoline {
	// interpret the left-hand side
	return expr.Left.Accept(interpreter).(Trampoline).
		FlatMap(func(left interface{}) Trampoline {
			// after interpreting the left-hand side,
			// interpret the right-hand side
			return expr.Right.Accept(interpreter).(Trampoline).
				FlatMap(func(right interface{}) Trampoline {
					tuple := TupleValue{
						left.(Value),
						right.(Value),
					}
					return Done{Result: tuple}
				})
		})
}

func (interpreter *Interpreter) VisitBinaryExpression(expression *ast.BinaryExpression) ast.Repr {
	switch expression.Operation {
	case ast.OperationPlus:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.Plus(right)
			})

	case ast.OperationMinus:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.Minus(right)
			})

	case ast.OperationMod:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.Mod(right)
			})

	case ast.OperationMul:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.Mul(right)
			})

	case ast.OperationDiv:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.Div(right)
			})

	case ast.OperationLess:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.Less(right)
			})

	case ast.OperationLessEqual:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.LessEqual(right)
			})

	case ast.OperationGreater:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.Greater(right)
			})

	case ast.OperationGreaterEqual:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(IntegerValue)
				right := tuple.right.(IntegerValue)
				return left.GreaterEqual(right)
			})

	case ast.OperationEqual:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)

				switch left := tuple.left.(type) {
				case IntegerValue:
					right := tuple.right.(IntegerValue)
					return BoolValue(left.Equal(right))

				case BoolValue:
					return BoolValue(tuple.left == tuple.right)
				}

				panic(&errors.UnreachableError{})
			})

	case ast.OperationUnequal:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)

				switch left := tuple.left.(type) {
				case IntegerValue:
					right := tuple.right.(IntegerValue)
					return BoolValue(!left.Equal(right))

				case BoolValue:
					return BoolValue(tuple.left != tuple.right)
				}

				panic(&errors.UnreachableError{})
			})

	case ast.OperationOr:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(BoolValue)
				right := tuple.right.(BoolValue)
				return BoolValue(left || right)
			})

	case ast.OperationAnd:
		return interpreter.visitBinaryOperation(expression).
			Map(func(result interface{}) interface{} {
				tuple := result.(TupleValue)
				left := tuple.left.(BoolValue)
				right := tuple.right.(BoolValue)
				return BoolValue(left && right)
			})
	}

	panic(&unsupportedOperation{
		kind:      common.OperationKindBinary,
		operation: expression.Operation,
		startPos:  expression.StartPosition(),
		endPos:    expression.EndPosition(),
	})
}

func (interpreter *Interpreter) VisitUnaryExpression(expression *ast.UnaryExpression) ast.Repr {
	return expression.Expression.Accept(interpreter).(Trampoline).
		Map(func(result interface{}) interface{} {
			value := result.(Value)

			switch expression.Operation {
			case ast.OperationNegate:
				boolValue := value.(BoolValue)
				return boolValue.Negate()

			case ast.OperationMinus:
				integerValue := value.(IntegerValue)
				return integerValue.Negate()
			}

			panic(&unsupportedOperation{
				kind:      common.OperationKindUnary,
				operation: expression.Operation,
				startPos:  expression.StartPos,
				endPos:    expression.EndPos,
			})
		})
}

func (interpreter *Interpreter) VisitExpressionStatement(statement *ast.ExpressionStatement) ast.Repr {
	return statement.Expression.Accept(interpreter).(Trampoline).
		Map(func(_ interface{}) interface{} {
			// NOTE: ignore result, so it does *not* act like a return-statement
			return nil
		})
}

func (interpreter *Interpreter) VisitBoolExpression(expression *ast.BoolExpression) ast.Repr {
	value := BoolValue(expression.Value)

	return Done{Result: value}
}

func (interpreter *Interpreter) VisitIntExpression(expression *ast.IntExpression) ast.Repr {
	value := IntValue{expression.Value}

	return Done{Result: value}
}

func (interpreter *Interpreter) VisitArrayExpression(expression *ast.ArrayExpression) ast.Repr {
	return interpreter.visitExpressions(expression.Values, nil)
}

func (interpreter *Interpreter) VisitMemberExpression(expression *ast.MemberExpression) ast.Repr {
	return expression.Expression.Accept(interpreter).(Trampoline).
		Map(func(result interface{}) interface{} {
			structure := result.(*StructureValue)
			return structure.Members[expression.Identifier]
		})
}

func (interpreter *Interpreter) VisitIndexExpression(expression *ast.IndexExpression) ast.Repr {
	return expression.Expression.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			array := result.(ArrayValue)

			return expression.Index.Accept(interpreter).(Trampoline).
				FlatMap(func(result interface{}) Trampoline {
					index := result.(IntegerValue)
					value := array[index.IntValue()]

					return Done{Result: value}
				})
		})
}

func (interpreter *Interpreter) VisitConditionalExpression(expression *ast.ConditionalExpression) ast.Repr {
	return expression.Test.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			value := result.(BoolValue)

			if value {
				return expression.Then.Accept(interpreter).(Trampoline)
			} else {
				return expression.Else.Accept(interpreter).(Trampoline)
			}
		})
}

func (interpreter *Interpreter) VisitInvocationExpression(invocationExpression *ast.InvocationExpression) ast.Repr {
	// interpret the invoked expression
	return invocationExpression.InvokedExpression.Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			function := result.(FunctionValue)

			// NOTE: evaluate all argument expressions in call-site scope, not in function body
			argumentExpressions := make([]ast.Expression, len(invocationExpression.Arguments))
			for i, argument := range invocationExpression.Arguments {
				argumentExpressions[i] = argument.Expression
			}

			return interpreter.visitExpressions(argumentExpressions, nil).
				FlatMap(func(result interface{}) Trampoline {
					arguments := result.(ArrayValue)
					return function.invoke(interpreter, arguments)
				})
		})
}

func (interpreter *Interpreter) invokeInterpretedFunction(
	function InterpretedFunctionValue,
	arguments []Value,
) Trampoline {

	// start a new activation record
	// lexical scope: use the function declaration's activation record,
	// not the current one (which would be dynamic scope)
	interpreter.activations.Push(function.Activation)

	return interpreter.invokeInterpretedFunctionActivated(function, arguments)
}

// NOTE: assumes the function's activation (or an extension of it) is pushed!
//
func (interpreter *Interpreter) invokeInterpretedFunctionActivated(
	function InterpretedFunctionValue,
	arguments []Value,
) Trampoline {

	interpreter.bindFunctionInvocationParameters(function, arguments)

	functionBlockTrampoline := interpreter.visitFunctionBlock(
		function.Expression.FunctionBlock,
		function.Expression.ReturnType,
	)

	return functionBlockTrampoline.
		Then(func(_ interface{}) {
			interpreter.activations.Pop()
		})
}

// bindFunctionInvocationParameters binds the argument values to the parameters in the function
func (interpreter *Interpreter) bindFunctionInvocationParameters(
	function InterpretedFunctionValue,
	arguments []Value,
) {
	for parameterIndex, parameter := range function.Expression.Parameters {
		argument := arguments[parameterIndex]
		interpreter.declareVariable(parameter.Identifier, argument)
	}
}

func (interpreter *Interpreter) visitExpressions(expressions []ast.Expression, values []Value) Trampoline {
	count := len(expressions)

	// no expressions? stop
	if count == 0 {
		return Done{Result: ArrayValue(values)}
	}

	// interpret the first expression
	return expressions[0].Accept(interpreter).(Trampoline).
		FlatMap(func(result interface{}) Trampoline {
			value := result.(Value)

			// interpret the remaining expressions
			return interpreter.visitExpressions(expressions[1:], append(values, value))
		})
}

func (interpreter *Interpreter) VisitFunctionExpression(expression *ast.FunctionExpression) ast.Repr {

	// lexical scope: variables in functions are bound to what is visible at declaration time
	lexicalScope := interpreter.activations.CurrentOrNew()

	function := newInterpretedFunction(expression, lexicalScope)

	return Done{Result: function}
}

func (interpreter *Interpreter) VisitStructureDeclaration(declaration *ast.StructureDeclaration) ast.Repr {
	constructorVariable := interpreter.structureConstructorVariable(declaration)

	// declare the constructor in the current scope
	interpreter.setVariable(declaration.Identifier, constructorVariable)

	// NOTE: no result, so it does *not* act like a return-statement
	return Done{}
}

// structureConstructorVariable creates a constructor function
// for the given structure, bound in a variable.
//
// The constructor is a host function which creates a new structure,
// calls the initializer (interpreted function), if any,
// and then returns the structure.
//
// Inside the initializer and all functions, `self` is bound to
// the new structure value, and the constructor itself is bound
//
func (interpreter *Interpreter) structureConstructorVariable(declaration *ast.StructureDeclaration) *Variable {

	// lexical scope: variables in functions are bound to what is visible at declaration time
	lexicalScope := interpreter.activations.CurrentOrNew()

	initializer := declaration.Initializer

	var initializerFunction *InterpretedFunctionValue
	if initializer != nil {
		functionExpression := initializer.ToFunctionExpression()
		function := newInterpretedFunction(functionExpression, lexicalScope)
		initializerFunction = &function
	}

	constructorVariable := &Variable{}

	functions := interpreter.structureFunctions(declaration, lexicalScope)

	// TODO: function type
	constructorVariable.Value = NewHostFunction(
		nil,
		func(interpreter *Interpreter, values []Value) Trampoline {
			structure := newStructure()

			for name, function := range functions {
				// NOTE: rebind, as function is captured in closure
				function := function

				// TODO: function type
				structure.Members[name] =
					NewHostFunction(
						nil,
						func(interpreter *Interpreter, values []Value) Trampoline {
							return interpreter.invokeStructureFunction(
								function,
								values,
								structure,
								declaration.Identifier,
								constructorVariable,
							)
						},
					)
			}

			var initializationTrampoline Trampoline = Done{}

			if initializerFunction != nil {
				initializationTrampoline = interpreter.invokeStructureFunction(
					*initializerFunction,
					values,
					structure,
					declaration.Identifier,
					constructorVariable,
				)
			}

			return initializationTrampoline.
				Map(func(_ interface{}) interface{} {
					return structure
				})
		},
	)

	return constructorVariable
}

// invokeStructureFunction calls the given function with the values.
//
// Inside the function, `self` is bound to the structure,
// and the constructor for the structure is bound
//
func (interpreter *Interpreter) invokeStructureFunction(
	function InterpretedFunctionValue,
	values []Value,
	structure *StructureValue,
	identifier string,
	constructorVariable *Variable,
) Trampoline {
	// start a new activation record
	// lexical scope: use the function declaration's activation record,
	// not the current one (which would be dynamic scope)
	interpreter.activations.Push(function.Activation)

	// make `self` available in the initializer
	interpreter.declareVariable(sema.SelfIdentifier, structure)

	// make the constructor available in the initializer
	interpreter.setVariable(identifier, constructorVariable)

	return interpreter.invokeInterpretedFunctionActivated(function, values)
}

func (interpreter *Interpreter) structureFunctions(
	declaration *ast.StructureDeclaration,
	lexicalScope hamt.Map,
) map[string]InterpretedFunctionValue {

	functions := map[string]InterpretedFunctionValue{}

	for _, functionDeclaration := range declaration.Functions {
		function := functionDeclaration.ToExpression()
		functions[functionDeclaration.Identifier] =
			newInterpretedFunction(function, lexicalScope)
	}

	return functions
}

func (interpreter *Interpreter) VisitFieldDeclaration(field *ast.FieldDeclaration) ast.Repr {
	panic(&errors.UnreachableError{})
}

func (interpreter *Interpreter) VisitInitializerDeclaration(initializer *ast.InitializerDeclaration) ast.Repr {
	panic(&errors.UnreachableError{})
}
