#!/usr/bin/env python3
import sys
import uuid
import re

def parse_args(s):
    """
    Parses a string that should contain exactly two arguments.
    Arguments might contain nested <spx-uuidv5 ...> tags.
    """
    args = []
    current_arg = []
    depth = 0
    i = 0
    while i < len(s):
        char = s[i]
        if char == '<':
            depth += 1
            current_arg.append(char)
        elif char == '>':
            depth -= 1
            current_arg.append(char)
        elif char.isspace() and depth == 0:
            if current_arg:
                args.append("".join(current_arg))
                current_arg = []
        else:
            current_arg.append(char)
        i += 1
    
    if current_arg:
        args.append("".join(current_arg))
    
    return args

def resolve_text(text):
    """
    Finds top-level <spx-uuidv5 ...> tags in text, evaluates them, 
    and returns the text with tags replaced by their results.
    """
    result = []
    i = 0
    while i < len(text):
        start_idx = text.find("<spx-uuidv5 ", i)
        if start_idx == -1:
            result.append(text[i:])
            break
        
        result.append(text[i:start_idx])
        
        # Find matching closing '>'
        depth = 0
        end_idx = -1
        for j in range(start_idx, len(text)):
            if text[j] == '<':
                depth += 1
            elif text[j] == '>':
                depth -= 1
                if depth == 0:
                    end_idx = j
                    break
        
        if end_idx != -1:
            content = text[start_idx + len("<spx-uuidv5 "):end_idx]
            val = process_tag(content)
            result.append(val)
            i = end_idx + 1
        else:
            # Unbalanced tag
            result.append(text[start_idx])
            i = start_idx + 1
            
    return "".join(result)

def process_tag(content):
    """
    Processes the content of a <spx-uuidv5 ...> tag.
    Returns the resulting UUIDv5 string.
    """
    args = parse_args(content)
    if len(args) != 2:
        # Fallback if args are weird, though the prompt says arg1 and arg2
        # We'll just return it as is or handle it
        return f"<invalid-v5 {content}>"

    # Resolve each argument recursively
    ns_str = resolve_text(args[0])
    name = resolve_text(args[1])
    
    try:
        namespace = uuid.UUID(ns_str)
        u5 = uuid.uuid5(namespace, name)
        res = str(u5)
        return res
    except Exception as e:
        # If arg1 is not a valid UUID, we can't calculate UUIDv5
        return f"<error-{ns_str}>"

if __name__ == "__main__":
    for line in sys.stdin:
        # We use end="" because line already includes newline from sys.stdin
        # or we strip it and add it back.
        # Let's keep the original line ending if possible.
        # If we use line.strip() we might lose trailing whitespace.
        processed = resolve_text(line.rstrip('\n'))
        print(processed)

