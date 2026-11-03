package elseoneline

# A one-line else without braces: OPA gives its body the location of the whole clause, which regodrill
# corrects, so these comparisons are mutated like any other.
tier := "gold" if input.points >= 100 else := "silver" if input.points >= 50 else := "bronze"
