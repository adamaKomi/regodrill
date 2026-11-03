package stillborn

# EXPR_REMOVE skips an expression that declares a variable the rest of the rule needs: that mutant could
# only fail to compile. When another expression could bind the variable, the mutant is kept.
title := t if {
	t := input.title # no mutant: t is used in the head and nothing else binds it
	count(t) > 0 # > → >= survives: no test has an empty title
}

label := value if {
	key := input.key # kept: input.labels[key] can bind key by iteration; killed (two labels, two values)
	guard := input.guard # kept: guard is not used elsewhere, so this line is a guard (survives, untested)
	value := input.labels[key] # no mutant: value is used in the head
}

annotation := a if {
	name := input.default_name # kept: input.annotations[name] can bind name by iteration (killed)
	a := input.annotations[name] # no mutant: a is used in the head
}

level := l if {
	l = input.level # kept but stillborn: `=` is not a declaration, so the check leaves it to the compiler
	l > 0 # removal and > → >= survive: no test uses level 0
}
