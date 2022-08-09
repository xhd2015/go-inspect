# How to refactor a large and long process function to provide flexible customization to the user?
The problem is about refactoring. To refactor, we have the following tools:
- Patterns
- Recognize the nature of the problem, and reorganize

The original large process function relies on some parameters, and some steps.

So the first is to try to propose at least 2 or more scenarios that will fit into this process, and find what is shared among these usages.

This is a down-to-top solution.

# Refactor
Closing ranges, shrink size of the target function.

Provide chances for user to customize some process

Make these customization reasonable, and simple.