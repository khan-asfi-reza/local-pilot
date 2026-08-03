(function_item name: (identifier) @name.definition.function) @definition.function
(struct_item name: (type_identifier) @name.definition.type) @definition.type
(enum_item name: (type_identifier) @name.definition.type) @definition.type
(trait_item name: (type_identifier) @name.definition.type) @definition.type
(call_expression function: (identifier) @name.reference.call)
