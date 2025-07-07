import argparse
import subprocess
import zipfile
import shutil
from pathlib import Path
from datetime import datetime

class CodebaseSwitcher:
    STATES = ['preedit', 'beetle', 'sonnet', 'rewrite']
    ACTIVE_STATES = ['beetle', 'sonnet', 'rewrite']
    EXCLUDE_FILES = {'codebase_switcher_mi.py', '.gitignore'}
    
    EXCLUDE_PATTERNS = {
        'node_modules', '.npm', '.yarn', 'npm-debug.log', 'yarn-debug.log', 'yarn-error.log',
        '.pnpm-debug.log', '.next', '.nuxt', '.vuepress', 'target', '.gradle', 'bin', 'obj',
        '__pycache__', '.venv', 'venv', 'ENV', '.Python', 'develop-eggs', 'downloads', 
        'eggs', '.eggs', 'lib64', 'parts', 'sdist', 'var', 'wheels',
        '.vscode', '.idea', '.sublime-project', '.sublime-workspace', '.DS_Store', 
        '.Spotlight-V100', '.Trashes', 'ehthumbs.db', 'Thumbs.db', '.cache', '.temp', '.tmp', 'logs'
    }
    
    EXCLUDE_EXTENSIONS = {
        '.pyc', '.pyo', '.class', '.o', '.a', '.lib', '.so', '.dylib', '.dll', '.exe', '.pdb',
        '.log', '.tmp', '.temp', '.swp', '.swo', '.bak', '.backup', '.old', '.orig', '.save',
        '.zip', '.tar', '.tar.gz', '.rar', '.7z', '.bz2', '.xz', '.deb', '.rpm', '.pkg', 
        '.dmg', '.msi', '.war', '.ear'
    }
    
    GIT_USER = 'Codebase Switcher'
    GIT_EMAIL = 'switcher@codebase.local'
    
    MESSAGES = {
        'preedit_ready': "✅ Already on preedit baseline branch",
        'preedit_now': "✅ Now on preedit baseline branch",
        'model_ready': "✅ Already on {state} branch\n🤖 Ready for {title} model work",
        'model_now': "✅ Now on {state} branch\n🤖 Ready for {title} model work",
        'rewrite_ready': "✅ Already on rewrite branch\n🔄 Ready for rewrite work",
        'rewrite_now': "✅ Now on rewrite branch\n🔄 Ready for rewrite work"
    }
    
    def __init__(self):
        self.current_dir = Path.cwd()
        self.base_branch = None
        self.keep_branch = None
        self._current_branch_cache = None
        self._git_available = None
        
    def _run_git(self, args, capture=False):
        try:
            result = subprocess.run(['git'] + args, capture_output=capture, text=True, 
                                  check=not capture, cwd=self.current_dir)
            return result.stdout.strip() if capture and result.returncode == 0 else (True if not capture else None)
        except (FileNotFoundError, subprocess.CalledProcessError) as e:
            if not capture and hasattr(e, 'returncode') and e.returncode == 128:
                print(f"❌ Git error: {getattr(e, 'stderr', 'Repository operation failed')}")
            return False if not capture else None
    
    def _validate_git(self):
        if self._git_available is None:
            if not shutil.which('git'):
                print("❌ Git not found in PATH.")
                self._git_available = False
            elif not (self.current_dir / '.git').exists():
                print("❌ Not a git repo. Run --init first.")
                self._git_available = False
            else:
                self._git_available = True
        return self._git_available
    
    def _get_current_branch(self):
        if self._current_branch_cache is None:
            self._current_branch_cache = self._run_git(['branch', '--show-current'], capture=True) or "main"
        return self._current_branch_cache
    
    def _clear_branch_cache(self):
        self._current_branch_cache = None
    
    def _branch_exists(self, branch):
        result = self._run_git(['branch', '--list', branch], capture=True)
        return branch in result if result else False
    
    def _has_changes(self):
        return bool(self._run_git(['status', '--porcelain'], capture=True))
    
    def _get_rewrite_base_branch(self):
        """Get the base branch that rewrite was created from"""
        result = self._run_git(['config', '--local', 'switcher.rewrite.base'], capture=True)
        return result if result else None
    
    def _set_rewrite_base_branch(self, base_branch):
        """Store the base branch that rewrite was created from"""
        return self._run_git(['config', '--local', 'switcher.rewrite.base', base_branch])
    
    def _rebase_rewrite_branch(self, new_base, old_base):
        """Rebase the rewrite branch from old_base to new_base"""
        current = self._get_current_branch()
        
        print(f"🔄 Rebasing rewrite branch from '{old_base}' to '{new_base}'...")
        
        # Ensure we're on rewrite branch
        if current != 'rewrite':
            if not self._handle_commit_before_switch(current):
                return False
            if not self._switch_branch('rewrite'):
                return False
        
        # Commit any pending changes in rewrite
        if self._has_changes():
            if not self._auto_commit(" - before rebase"):
                print("❌ Failed to commit changes before rebase")
                return False
        
        # Perform the rebase
        print(f"🔀 Executing: git rebase --onto {new_base} {old_base} rewrite")
        if not self._run_git(['rebase', '--onto', new_base, old_base, 'rewrite']):
            print(f"❌ Rebase failed! You may need to resolve conflicts manually.")
            print(f"   Use 'git rebase --continue' after resolving conflicts")
            print(f"   Or use 'git rebase --abort' to cancel the rebase")
            return False
        
        # Update the stored base branch
        if not self._set_rewrite_base_branch(new_base):
            print("⚠️  Warning: Could not update base branch config")
        
        self._clear_branch_cache()
        print(f"✅ Successfully rebased rewrite branch to '{new_base}'")
        print("🔄 Ready for rewrite work on new base")
        return True
    
    def _should_exclude(self, file_path, zip_name):
        name = file_path.name
        
        # Check exact file matches
        if name in self.EXCLUDE_FILES or name == zip_name:
            return True
        
        # Check excluded extensions
        if file_path.suffix.lower() in self.EXCLUDE_EXTENSIONS:
            return True
        
        # Check excluded patterns in path
        return any(
            part in self.EXCLUDE_PATTERNS or name in self.EXCLUDE_PATTERNS or
            any(name.startswith(pattern) for pattern in self.EXCLUDE_PATTERNS if not pattern.startswith('.'))
            for part in file_path.parts
        )
    
    def _auto_commit(self, suffix=""):
        if not self._has_changes():
            return True
        current = self._get_current_branch()
        print(f"💾 Auto-committing changes on {current}{suffix}")
        return self._run_git(['add', '.']) and self._run_git(['commit', '-m', f'Auto-commit on {current}{suffix}'])
    
    def _handle_commit_before_switch(self, current):
        if not self._has_changes():
            return True
        
        if current in self.ACTIVE_STATES:
            return self._auto_commit(f" - {current} model work")
        elif current == 'preedit':
            print("📝 Switching from baseline (changes will be preserved)")
            return True
        return True
    
    def _switch_branch(self, target):
        print(f"🔄 Switching from {self._get_current_branch()} to {target} branch...")
        if self._run_git(['checkout', target]):
            self._clear_branch_cache()
            return True
        print(f"❌ Failed to switch to {target}")
        return False
    
    def _print_ready_message(self, state):
        """Print message when already on target state"""
        if state == 'preedit':
            print(self.MESSAGES['preedit_ready'])
        elif state == 'rewrite':
            print(self.MESSAGES['rewrite_ready'])
        else:
            print(self.MESSAGES['model_ready'].format(state=state, title=state.title()))
    
    def _print_status_message(self, state):
        """Print message when switching to target state"""
        if state == 'preedit':
            print(self.MESSAGES['preedit_now'])
        elif state == 'rewrite':
            print(self.MESSAGES['rewrite_now'])
        else:
            print(self.MESSAGES['model_now'].format(state=state, title=state.title()))
    
    def _create_gitignore_patterns(self):
        patterns = []
        for pattern in self.EXCLUDE_PATTERNS:
            if pattern.startswith('.'):
                patterns.extend([pattern, f"**/{pattern}"])
            else:
                patterns.extend([f"{pattern}/", f"**/{pattern}/"])
        
        # Add extensions
        for ext in self.EXCLUDE_EXTENSIONS:
            patterns.append(f"*{ext}")
        return patterns
    
    def initialize(self):
        if not shutil.which('git'):
            print("❌ Git not found in PATH.")
            return False
            
        if not (self.current_dir / '.git').exists():
            print("🔧 Initializing git repository...")
            if not self._run_git(['init']):
                return False
            print("🔧 Setting up local git configuration...")
            self._run_git(['config', '--local', 'user.name', self.GIT_USER])
            self._run_git(['config', '--local', 'user.email', self.GIT_EMAIL])
        
        # Create/update .gitignore BEFORE any commits
        self._create_gitignore()
        
        if not self._run_git(['log', '--oneline', '-1'], capture=True):
            print("📝 Creating initial commit...")
            self._create_initial_files()
            
            print("\n⚠️  IMPORTANT: Please review the generated .gitignore file!")
            print("   Add any additional patterns for your project's build artifacts.")
            print("   Press Enter when ready to continue with initial commit...")
            input()
            
            if not (self._run_git(['add', '.']) and 
                   self._run_git(['commit', '-m', 'Initial commit - baseline for model comparison'])):
                print("❌ Failed to create initial commit")
                return False
        
        if not (self._auto_commit(" before branch setup") and self._create_branches()):
            return False
        
        print("✅ Initialized model comparison environment")
        return True
    
    def _create_gitignore(self):
        """Create or update .gitignore with appropriate patterns"""
        gitignore_path = self.current_dir / '.gitignore'
        patterns = self._create_gitignore_patterns()
        switcher_section = ("# Auto-generated by codebase switcher\n"
                           "# Excludes large files and build artifacts to keep branches clean\n\n" +
                           '\n'.join(patterns) + "\n")
        
        if gitignore_path.exists():
            existing = gitignore_path.read_text(encoding='utf-8')
            if "# Auto-generated by codebase switcher" not in existing:
                gitignore_path.write_text(existing.rstrip() + "\n\n" + switcher_section, encoding='utf-8')
                print("📋 Updated existing .gitignore with switcher patterns")
            else:
                print("📋 .gitignore already contains switcher patterns")
        else:
            gitignore_path.write_text(switcher_section, encoding='utf-8')
            print("📋 Created .gitignore with switcher patterns")
    
    def _create_initial_files(self):
        """Create initial files, preserving existing README if present"""
        try:
            readme_path = self.current_dir / 'README.md'
            
            # Preserve existing README by backing it up and appending model comparison info
            if readme_path.exists():
                existing_readme = readme_path.read_text(encoding='utf-8')
                print("📖 Found existing README.md - preserving original content")
                
                model_comparison_section = (
                    f"\n\n---\n\n## Model Comparison Project\n\n"
                    f"**Project Date:** {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n\n"
                    f"This project contains model comparison results using codebase switcher:\n\n"
                    f"- `preedit` branch: Original baseline codebase\n"
                    f"- `beetle` branch: Beetle model's response\n"
                    f"- `sonnet` branch: Sonnet model's response\n"
                    f"- `rewrite` branch: Rewritten codebase\n"
                )
                
                # Only append if not already present
                if "## Model Comparison Project" not in existing_readme:
                    readme_path.write_text(existing_readme + model_comparison_section, encoding='utf-8')
                    print("📖 Appended model comparison info to existing README.md")
                else:
                    print("📖 README.md already contains model comparison info")
            else:
                # Create new README only if none exists
                readme_content = (f"# Model Comparison Project\n\n"
                                 f"**Project Date:** {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}\n\n"
                                 f"This project contains model comparison results.\n\n"
                                 f"- `preedit` branch: Original baseline codebase\n"
                                 f"- `beetle` branch: Beetle model's response\n"
                                 f"- `sonnet` branch: Sonnet model's response\n"
                                 f"- `rewrite` branch: Rewritten codebase\n")
                readme_path.write_text(readme_content, encoding='utf-8')
                print("📖 Created new README.md with model comparison info")
                
        except (OSError, UnicodeError) as e:
            print(f"⚠️  Warning: Could not handle README file: {e}")
    
    def _create_branches(self):
        # Ensure preedit branch exists and switch to it
        if not self._ensure_branch_with_changes_handled('preedit', create_from='HEAD'):
            return False
        
        # Create model branches from preedit
        for state in ['beetle', 'sonnet']:
            if not self._branch_exists(state):
                if not self._ensure_branch_with_changes_handled(state, create_from='preedit'):
                    return False
            if not self._switch_branch('preedit'):
                return False
        return True
    
    def switch_state(self, state):
        if state not in self.STATES:
            print(f"❌ Invalid state. Available: {', '.join(self.STATES)}")
            return False
        
        if not self._validate_git():
            return False
        
        if state == 'rewrite':
            return self._handle_rewrite()
        
        current = self._get_current_branch()
        
        # Handle already on target state
        if current == state:
            if current in self.ACTIVE_STATES and self._has_changes():
                if not self._auto_commit(f" - {state} model work"):
                    return False
            self._print_ready_message(state)
            return True
        
        # Switch to target state
        if self._ensure_branch_with_changes_handled(state):
            self._print_status_message(state)
            return True
        return False
    
    def _handle_rewrite(self):
        base_branch = self.base_branch or 'preedit'
        
        if not self._branch_exists(base_branch):
            print(f"❌ Base branch '{base_branch}' doesn't exist. Run --init first.")
            return False
        
        current = self._get_current_branch()
        
        if self._branch_exists('rewrite'):
            # Check if user wants to rebase to a different base branch
            current_base = self._get_rewrite_base_branch()
            if self.base_branch and current_base and current_base != base_branch:
                print(f"🔍 Rewrite branch currently based on '{current_base}' branch")
                print(f"📝 You specified '--base-branch={base_branch}' - this will rebase rewrite to '{base_branch}'")
                print("⚠️  WARNING: This will rewrite git history of the rewrite branch!")
                print("   Type 'REBASE' to proceed, or anything else to cancel:")
                
                confirmation = input().strip()
                if confirmation != 'REBASE':
                    print("❌ Rebase cancelled - staying on current rewrite branch")
                    if current != 'rewrite':
                        return self._switch_branch('rewrite') and (print(self.MESSAGES['rewrite_now']) or True)
                    return True
                
                return self._rebase_rewrite_branch(base_branch, current_base)
            
            if current == 'rewrite':
                if self._has_changes() and not self._auto_commit(" - rewrite work"):
                    return False
                print(self.MESSAGES['rewrite_ready'])
                return True
            
            if not self._handle_commit_before_switch(current):
                return False
            
            return self._switch_branch('rewrite') and (print(self.MESSAGES['rewrite_now']) or True)
        
        print(f"📝 Creating rewrite branch from {base_branch} branch...")
        
        if not self._handle_commit_before_switch(current):
            return False
        
        if current == 'preedit' and self._has_changes():
            if not self._auto_commit(" - baseline changes"):
                return False
        
        if current != base_branch and not self._switch_branch(base_branch):
            return False
        
        if self._run_git(['checkout', '-b', 'rewrite']):
            self._clear_branch_cache()
            # Store the base branch for future reference
            if not self._set_rewrite_base_branch(base_branch):
                print("⚠️  Warning: Could not store rewrite base branch config")
            print(f"✅ Created rewrite branch from {base_branch}\n🔄 Ready for rewrite work")
            return True
        print("❌ Failed to create rewrite branch")
        return False
    
    def show_status(self):
        if not self._validate_git():
            return
        
        current = self._get_current_branch()
        has_changes = self._has_changes()
        
        print(f"📍 Current branch: {current}")
        print(f"🔄 Available states: preedit (baseline), beetle, sonnet, rewrite")
        
        # Branch-specific status
        branch_info = {
            'preedit': {
                'description': "💻 On baseline branch",
                'changes_msg': "📝 Baseline has changes (will be preserved when switching)"
            },
            'default_active': {
                'description': lambda: f"🤖 On {current} branch - working with {current.title()}",
                'changes_msg': "⚡ You have uncommitted model work (will auto-commit on switch)"
            },
            'rewrite': {
                'description': "🤖 On rewrite branch - working with rewrite",
                'changes_msg': "⚡ You have uncommitted rewrite work (will auto-commit on switch)",
                'extra_info': lambda: self._get_rewrite_base_branch() and f"📍 Rewrite based on '{self._get_rewrite_base_branch()}' branch"
            }
        }
        
        if current == 'preedit':
            info = branch_info['preedit']
        elif current == 'rewrite':
            info = branch_info['rewrite']
        elif current in self.ACTIVE_STATES:
            info = branch_info['default_active']
        else:
            info = {'description': f"📍 On {current} branch", 'changes_msg': "⚡ You have uncommitted changes"}
        
        # Print description
        desc = info['description']() if callable(info['description']) else info['description']
        print(desc)
        
        # Print extra info if available
        if 'extra_info' in info and info['extra_info']:
            extra = info['extra_info']() if callable(info['extra_info']) else info['extra_info']
            if extra:
                print(extra)
        
        # Print changes status
        if has_changes:
            print(info['changes_msg'])
        else:
            print("✅ All changes committed")
        
        # Commands and git info
        print(f"\n💡 Quick commands:")
        print(f"   • Switch to baseline: python {Path(__file__).name} --preedit")
        print(f"   • Reset to original:  python {Path(__file__).name} --reset")
        
        git_sections = [
            ("🔄 Git status:", ['status', '--short'], "No changes"),
            ("📚 Recent commits:", ['log', '--oneline', '-3'], "No commits found")
        ]
        
        for section, cmd, empty_msg in git_sections:
            print(f"\n{section}")
            result = self._run_git(cmd, capture=True)
            print(result if result else empty_msg)
    
    def show_version(self):
        print("v.0701_1600_mi")
        print("🆕 Improvements:")
        print("   • Added --preedit option to return to baseline")
        print("   • Added --reset option for project reset")
        print("   • Preserves existing README files")
        print("   • Better .gitignore setup with review prompt")
        print("   • Support for rebasing rewrite to different base branches")
        print("   • Shows current rewrite base branch in status")
    
    def create_zip(self):
        if not self._validate_git():
            return False
        
        # Ensure all required branches are properly committed
        if not self._prepare_branches_for_zip():
            return False
        
        if not self._verify_branches_different():
            return False
        
        print("🔄 Switching to preedit baseline for zip creation...")
        if not self._switch_branch('preedit'):
            return False
        
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        zip_name = f"model_comparison_{self.current_dir.name}_{timestamp}.zip"
        zip_path = self.current_dir / zip_name
        
        print(f"📦 Creating model comparison results: {zip_name}...")
        
        # Create temporary directory and copy entire original repository
        import tempfile
        temp_dir = None
        try:
            temp_dir = Path(tempfile.mkdtemp(prefix="model_comparison_"))
            
            print("📋 Copying entire original repository (including .git with full history)...")
            
            # Simply copy the entire repository exactly as it is - NO git operations
            for file_path in self.current_dir.rglob('*'):
                if file_path.is_file() and not self._should_exclude(file_path, zip_name):
                    relative_path = file_path.relative_to(self.current_dir)
                    temp_file_path = temp_dir / relative_path
                    temp_file_path.parent.mkdir(parents=True, exist_ok=True)
                    try:
                        shutil.copy2(file_path, temp_file_path)
                    except (OSError, ValueError) as e:
                        print(f"⚠️  Skipping {file_path.name}: {e}")
            
            print("✅ Original repository copied with all branches and commit history preserved")
            
            # Create zip from the clean temporary repository
            with zipfile.ZipFile(zip_path, 'w', zipfile.ZIP_DEFLATED) as zipf:
                for file_path in temp_dir.rglob('*'):
                    if file_path.is_file():
                        relative_path = file_path.relative_to(temp_dir)
                        zipf.write(file_path, relative_path.as_posix())
            
            print(f"✅ Created {zip_name} ({zip_path.stat().st_size / 1024:.1f} KB)")
            print(f"📋 Contains: Complete original repository with all branches and full git history")
            
            final_branch = self._determine_final_branch()
            
            print("\n🧹 Starting complete branch cleanup...")
            success = self._cleanup_branches(final_branch)
            
            final_content = final_branch
            print("📊 Zip created and branches cleaned up successfully!\n🎯 Project ready for fresh analysis or archival" 
                  if success else "⚠️  Zip created but branch cleanup had issues\n   You may need to manually clean up remaining branches")
            print(f"📍 Final codebase contains: {final_content} branch content")
            return success
            
        except Exception as e:
            print(f"❌ Error creating zip: {e}")
            if zip_path.exists():
                try:
                    zip_path.unlink()
                    print("🗑️  Cleaned up incomplete zip file")
                except OSError:
                    print("⚠️  Could not clean up incomplete zip file")
            return False
        finally:
            # Clean up temporary directory
            if temp_dir and temp_dir.exists():
                try:
                    shutil.rmtree(temp_dir)
                except OSError:
                    print(f"⚠️  Could not clean up temporary directory: {temp_dir}")
    
    def _prepare_branches_for_zip(self):
        """Ensure all target branches are properly committed before zip creation"""
        print("🔄 Preparing branches for zip creation...")
        
        target_branches = ['preedit', 'beetle', 'sonnet', 'rewrite']
        
        for branch in target_branches:
            if not self._branch_exists(branch):
                print(f"⚠️  Branch '{branch}' doesn't exist, skipping...")
                continue
            
            def prepare_branch():
                if self._has_changes():
                    if branch == 'preedit':
                        print(f"🧹 Discarding uncommitted changes in {branch} (keeping original baseline)")
                        if not self._run_git(['reset', '--hard', 'HEAD']):
                            raise Exception(f"Failed to discard changes in {branch}")
                        print(f"✅ {branch} reset to original baseline")
                    else:
                        print(f"💾 Auto-committing changes in {branch}")
                        if not self._auto_commit(f" - final {branch} work"):
                            raise Exception(f"Failed to commit changes in {branch}")
                        print(f"✅ {branch} changes committed")
                else:
                    print(f"✅ {branch} already clean")
                return True
            
            if not self._safe_branch_operation(prepare_branch, branch, f"Checking {branch} branch"):
                return False
        
        print("✅ All branches prepared for zip creation")
        return True

    def _determine_final_branch(self):
        """Determine and prepare the final branch for the codebase"""
        final_branch = self.keep_branch or 'preedit'
        
        if final_branch != 'preedit':
            if self._branch_exists(final_branch):
                print(f"🔄 Preparing {final_branch} branch content as final codebase...")
                if not self._switch_branch(final_branch):
                    print(f"⚠️  Warning: Could not switch to {final_branch}, keeping preedit")
                    final_branch = 'preedit'
            else:
                print(f"⚠️  Warning: Branch '{final_branch}' not found, keeping preedit")
                final_branch = 'preedit'
        
        return final_branch

    def _verify_branches_different(self):
        print("🔍 Verifying model results are different...")
        
        existing_branches = [state for state in self.ACTIVE_STATES if self._branch_exists(state)]
        
        if len(existing_branches) < 2:
            print(f"❌ Need at least 2 branches for comparison. Found: {', '.join(existing_branches) if existing_branches else 'none'}")
            print("   Complete work on model branches first.")
            return False
        
        print(f"📊 Comparing branches: {', '.join(existing_branches)}")
        
        try:
            all_same = True
            for i, branch1 in enumerate(existing_branches):
                for branch2 in existing_branches[i+1:]:
                    result = subprocess.run(['git', 'diff', '--quiet', branch1, branch2], 
                                          cwd=self.current_dir, capture_output=True, check=False)
                    if result.returncode != 0:
                        all_same = False
                        print(f"✅ {branch1} and {branch2} branches are different")
                    else:
                        print(f"⚠️  Warning: {branch1} and {branch2} branches are identical")
            
            if all_same:
                print("⚠️  All branches are identical - this suggests similar responses")
                print("   This is still valid comparison data")
            else:
                print("✅ Model/rewrite responses show differences - good comparison data")
            return True
            
        except FileNotFoundError:
            print("❌ Git not found")
            return False
    
    def _cleanup_branches(self, keep_content_from=None):
        print("🧹 Cleaning up switcher branches...")
        
        safe_branch = next((branch for branch in ['main', 'master'] if self._branch_exists(branch)), None)
        current_branch = self._get_current_branch()
        
        if not safe_branch:
            print("🔄 Creating main branch as safe branch...")
            if not self._run_git(['checkout', '-b', 'main']):
                print("❌ Failed to create main branch")
                return False
            safe_branch = 'main'
        elif current_branch in self.STATES and (not keep_content_from or current_branch != keep_content_from):
            print(f"🔄 Switching to {safe_branch} branch...")
            if not self._switch_branch(safe_branch):
                return False
        elif current_branch in self.STATES and keep_content_from and current_branch == keep_content_from:

            print(f"🔄 Merging {keep_content_from} content to {safe_branch} branch...")
            if not self._switch_branch(safe_branch):
                return False

            if not self._run_git(['reset', '--hard', keep_content_from]):
                print(f"⚠️  Warning: Could not copy {keep_content_from} content to {safe_branch}")
        elif current_branch != safe_branch:
            print(f"🔄 Switching to {safe_branch} branch...")
            if not self._switch_branch(safe_branch):
                return False
        
        if self._get_current_branch() in self.STATES:
            print("❌ Still on switcher branch, cleanup aborted")
            return False
        
        success = True
        for state in self.STATES:
            if self._branch_exists(state):
                print(f"🗑️  Force deleting {state} branch...")
                if not self._run_git(['branch', '-D', state]):
                    print(f"❌ Failed to delete {state} branch")
                    success = False
                else:
                    print(f"✅ Successfully deleted {state} branch")
                    if state == 'rewrite':
                        self._run_git(['config', '--local', '--unset', 'switcher.rewrite.base'])
        
        remaining = [state for state in self.STATES if self._branch_exists(state)]
        if remaining:
            print(f"❌ Failed to delete branches: {', '.join(remaining)}")
            return False
        
        print("🧹 Cleaning up branch references...")
        self._run_git(['gc', '--prune=now'])
        print("✅ Cleanup complete - all switcher branches removed")
        print(f"📍 Now on {safe_branch} branch with model comparison results")
        return success

    def reset_to_baseline(self):
        """Reset the entire project back to the original preedit baseline"""
        if not self._validate_git():
            return False
            
        print("⚠️  WARNING: This will reset your project to the original preedit baseline!")
        print("   All work on beetle, sonnet, and rewrite branches will be lost.")
        print("   Type 'CONFIRM' to proceed, or anything else to cancel:")
        
        confirmation = input().strip()
        if confirmation != 'CONFIRM':
            print("❌ Reset cancelled")
            return False
            
        if not self._branch_exists('preedit'):
            print("❌ No preedit baseline found. Cannot reset.")
            return False
            
        current = self._get_current_branch()
        print(f"🔄 Resetting from current branch: {current}")
        
        # Force switch to preedit and reset any changes
        if not self._switch_branch('preedit'):
            print("❌ Failed to switch to preedit branch")
            return False
            
        # Reset any uncommitted changes in preedit
        if self._has_changes():
            print("🧹 Discarding any changes in preedit baseline")
            if not self._run_git(['reset', '--hard', 'HEAD']):
                print("❌ Failed to reset preedit changes")
                return False
        
        # Delete other branches if they exist
        for branch in ['beetle', 'sonnet', 'rewrite']:
            if self._branch_exists(branch):
                print(f"🗑️  Deleting {branch} branch")
                if not self._run_git(['branch', '-D', branch]):
                    print(f"⚠️  Warning: Could not delete {branch} branch")
                elif branch == 'rewrite':
                    # Clean up stored config for rewrite branch
                    self._run_git(['config', '--local', '--unset', 'switcher.rewrite.base'])
                    
        print("✅ Reset complete! Back to original preedit baseline")
        print("🔄 Run switcher commands to create new model branches as needed")
        return True

    def _ensure_branch_with_changes_handled(self, target_branch, create_from=None):
        """Ensure target branch exists and handle any current changes before switching"""
        if not self._branch_exists(target_branch):
            if create_from:
                print(f"📝 Creating {target_branch} branch from {create_from}")
                if not self._switch_branch(create_from):
                    return False
                return self._run_git(['checkout', '-b', target_branch])
            else:
                print(f"❌ Branch '{target_branch}' doesn't exist. Run --init first.")
                return False
        
        current = self._get_current_branch()
        if current == target_branch:
            return True
            
        if not self._handle_commit_before_switch(current):
            return False
            
        return self._switch_branch(target_branch)
    
    def _safe_branch_operation(self, operation, branch, description=""):
        """Safely perform branch operation with error handling"""
        print(f"🔄 {description or f'Operating on {branch} branch'}...")
        if not self._switch_branch(branch):
            print(f"❌ Failed to switch to {branch} branch")
            return False
        
        try:
            return operation()
        except Exception as e:
            print(f"❌ Operation failed on {branch}: {e}")
            return False

def main():
    parser = argparse.ArgumentParser(description='Model Comparison Tool - Experimentation & Comparison')
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('-i', '--init', action='store_true', help='Initialize model comparison environment')
    group.add_argument('-1', '--preedit', action='store_true', help='Switch to preedit baseline branch') 
    group.add_argument('-2', '--beetle', action='store_true', help='Switch to beetle branch (auto-commits model work)') 
    group.add_argument('-3', '--sonnet', action='store_true', help='Switch to sonnet branch (auto-commits model work)')
    group.add_argument('-4', '--rewrite', action='store_true', help='Switch to rewrite branch (auto-commits model work)')
    group.add_argument('-s', '--status', action='store_true', help='Show project status with guidance')
    group.add_argument('-z', '--zip', action='store_true', help='Create model comparison results zip and cleanup branches')
    group.add_argument('-r', '--reset', action='store_true', help='Reset project to original preedit baseline (DESTRUCTIVE)')
    group.add_argument('-v', '--version', action='store_true', help='Show version')
    
    parser.add_argument('--base-branch', choices=['preedit', 'beetle', 'sonnet'], 
                       help='Base branch for rewrite (default: preedit). Only used with --rewrite.')
    parser.add_argument('--keep-branch', choices=['preedit', 'beetle', 'sonnet', 'rewrite'], 
                       help='Branch content to keep as final codebase (default: preedit). Only used with --zip.')
    
    args = parser.parse_args()
    
    if args.base_branch and not args.rewrite:
        parser.error("--base-branch can only be used with --rewrite")
    
    if hasattr(args, 'keep_branch') and args.keep_branch and not args.zip:
        parser.error("--keep-branch can only be used with --zip")
    
    switcher = CodebaseSwitcher()
    if args.rewrite:
        switcher.base_branch = args.base_branch
    if args.zip:
        switcher.keep_branch = getattr(args, 'keep_branch', None)
    
    actions = {
        'init': switcher.initialize,
        'preedit': lambda: switcher.switch_state('preedit'),
        'beetle': lambda: switcher.switch_state('beetle'),
        'sonnet': lambda: switcher.switch_state('sonnet'), 
        'rewrite': lambda: switcher.switch_state('rewrite'),
        'status': switcher.show_status,
        'zip': switcher.create_zip,
        'reset': switcher.reset_to_baseline,
        'version': switcher.show_version
    }
    
    for action, func in actions.items():
        if getattr(args, action):
            func()
            break

if __name__ == '__main__':
    main() 