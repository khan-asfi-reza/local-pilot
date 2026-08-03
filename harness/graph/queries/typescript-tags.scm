(function_declaration name: (identifier) @name.definition.function) @definition.function
(class_declaration name: (type_identifier) @name.definition.class) @definition.class
(method_definition name: (property_identifier) @name.definition.method) @definition.method
(interface_declaration name: (type_identifier) @name.definition.type) @definition.type
(type_alias_declaration name: (type_identifier) @name.definition.type) @definition.type
(variable_declarator name: (identifier) @name.definition.function value: [(arrow_function) (function_expression)]) @definition.function
(call_expression function: (identifier) @name.reference.call)
(call_expression function: (member_expression property: (property_identifier) @name.reference.call))
