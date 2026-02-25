# 🛡️ ObfusPS - Secure PowerShell Script Protection

[![Download ObfusPS](https://img.shields.io/badge/Download-ObfusPS-blue?logo=github&style=for-the-badge)](https://github.com/mr-director/ObfusPS/releases)

---

## 📋 What is ObfusPS?

ObfusPS is a tool designed to protect PowerShell scripts. It hides the script's internal details without changing how the script works. This helps keep your scripts safe from copying or tampering.

ObfusPS runs on a modern engine made with the Go programming language. It does not need any extra software to work.

The tool changes your scripts in several layers. It analyzes your code smartly and makes changes that keep the script behavior exactly the same while making it very hard to read or understand.

---

## 💻 Who Should Use ObfusPS?

You do not need to be an expert to use ObfusPS. If you write or use PowerShell scripts and want to keep them private, this tool is for you. It works well for:

- System administrators protecting automation scripts.
- Security professionals who need to hide script logic.
- Developers who want to distribute their PowerShell tools securely.

No programming knowledge is required to get started.

---

## 🛠️ System Requirements

To run ObfusPS, your computer should meet these basic requirements:

- Operating System: Windows 10 or later, or any system that supports PowerShell and Go binaries.
- PowerShell: Version 5.1 or newer.
- Disk Space: At least 50 MB free.
- Memory: 4 GB RAM or higher.
- Internet connection to download the software.

---

## ⬇️ Download & Install ObfusPS

You can get ObfusPS from the official GitHub release page. This page contains the latest version and all past versions.

### How to download:

1. Click the big blue button at the top or visit this link:  
   [https://github.com/mr-director/ObfusPS/releases](https://github.com/mr-director/ObfusPS/releases)

2. On the releases page, look for the latest release. It will usually be at the top.

3. Download the file suited for your system. If you use Windows, this is often a `.exe` file or a zipped archive.

4. Save the file to a location you can easily find, like your Desktop or Downloads folder.

### How to install:

- If you downloaded a setup file (`.exe`), double-click it and follow the on-screen instructions.
- If you downloaded a zipped archive, right-click the file and select "Extract All" to unpack it.
- No extra steps or tools are needed because ObfusPS runs directly.

---

## ▶️ How to Use ObfusPS

Here is a simple way to run ObfusPS once it is installed, even if you have never used PowerShell before:

1. Find the ObfusPS program on your computer. If you installed it normally, it may appear in your Start Menu.
   
2. Open ObfusPS by double-clicking it.

3. You will see a window or command prompt asking for input.

4. Provide the PowerShell script you want to protect. This can be done by typing the file path or by dragging the file into the program.

5. Click the "Obfuscate" button or press Enter.

6. ObfusPS will process the script and create a new file. This new file contains your script but is hidden behind layers of protection.

7. The output file is saved in the same folder as the original unless you specify a different location.

---

## 🔍 Features Explained Simply

ObfusPS protects your PowerShell scripts using several steps:

- **Multi-layer obfuscation:** It changes your script many times over, making it very confusing to read.
- **Smart analysis:** ObfusPS looks at your script carefully before making changes. This ensures it won’t break.
- **AST-aware transforms:** The software understands the structure of your script, so the changes keep it working properly.
- **Optional runtime validation:** You can check that your script works the same way after protection.
- **No dependencies:** You do not need to install anything else for ObfusPS to work.
- **Fast and efficient:** ObfusPS runs quickly even on large scripts.

---

## ⚙️ Common Scenarios

- You created a script to automate tasks on your computer or network. You want others to use it but not see how it works.
- You work in cybersecurity and need to keep your scripts hidden to avoid detection.
- You want to share a PowerShell tool with teammates but keep the logic private.
- You want to prevent script copying or unauthorized edits.

In all these cases, ObfusPS helps by making the script unreadable while keeping it fully functional.

---

## 🧑‍🏫 Tips for Beginners

If you are new to PowerShell or software tools:

- Take your time to learn how to open and close files on your computer.
- Use the full path of your script file if you have trouble dragging files into ObfusPS. For example:  
  `C:\Users\YourName\Documents\script.ps1`
- Save your original script in a safe place before obfuscating.
- Test the obfuscated script by running it to make sure it behaves as expected.

---

## ❓ Troubleshooting

**Issue:** ObfusPS does not run.  
**Solution:** Make sure you met the system requirements and that your antivirus software does not block it.

**Issue:** The obfuscated script does not work.  
**Solution:** Try running the original script first. If it runs without problems, check if you used any unusual PowerShell features not supported yet.

**Issue:** I cannot find my obfuscated file.  
**Solution:** Look in the same folder where your original script is saved. Check ObfusPS settings for output location.

---

## 📖 Learn More

ObfusPS supports advanced options for expert users, such as:

- Runtime validation modes.
- Custom obfuscation levels.
- Script analysis reports.

For detailed developer use or advanced scenarios, visit the GitHub page or read the online documentation.

---

## 🔗 Useful Links

- ObfusPS Release Page:  
  [https://github.com/mr-director/ObfusPS/releases](https://github.com/mr-director/ObfusPS/releases)

- Official PowerShell Website:  
  [https://docs.microsoft.com/powershell](https://docs.microsoft.com/powershell)

- Basic PowerShell Tutorials:  
  [https://www.microsoft.com/learn/powershell](https://www.microsoft.com/learn/powershell)

---

## 📝 Acknowledgments

ObfusPS is built using the Go language for speed and portability. It aims to serve the cybersecurity and IT communities by providing a reliable PowerShell obfuscation tool without dependencies.

---

[![Download ObfusPS](https://img.shields.io/badge/Download-ObfusPS-blue?logo=github&style=for-the-badge)](https://github.com/mr-director/ObfusPS/releases)